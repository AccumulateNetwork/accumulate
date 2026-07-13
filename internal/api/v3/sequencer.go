// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Sequencer struct {
	logger      logging.OptionalLogger
	db          database.Viewer
	partitionID string
	partition   config.NetworkUrl
	valKey      []byte
	globals     atomic.Value

	// The pinned sync-epoch snapshot (#4058)
	snapMu sync.Mutex
	snap   *pinnedSnapshot

	// Recent block → (consensus round, committee epoch), from DidCommitBlock
	// events. A fast-syncing node needs its epoch block's round and epoch to
	// rejoin consensus; nothing else records the mapping (#4058).
	commitMu     sync.Mutex
	commitRounds map[uint64]blockCommit
	commitOldest uint64
}

type blockCommit struct {
	round uint64
	epoch uint64
}

// commitRoundRetention bounds how many block→round mappings are kept.
const commitRoundRetention = 1 << 14

var _ private.Sequencer = (*Sequencer)(nil)

type SequencerParams struct {
	Logger       logging.Logger
	Database     database.Viewer
	EventBus     *events.Bus
	Globals      *core.GlobalValues
	Partition    string
	ValidatorKey []byte
}

func NewSequencer(params SequencerParams) *Sequencer {
	s := new(Sequencer)
	s.logger.L = params.Logger
	s.db = params.Database
	s.partitionID = params.Partition
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	s.valKey = params.ValidatorKey
	s.globals.Store(params.Globals)
	events.SubscribeSync(params.EventBus, func(e events.WillChangeGlobals) error {
		s.globals.Store(e.New.Copy())
		return nil
	})
	s.commitRounds = map[uint64]blockCommit{}
	events.SubscribeSync(params.EventBus, func(e events.DidCommitBlock) error {
		if e.Round == 0 {
			return nil // CometBFT — no rounds
		}
		s.commitMu.Lock()
		defer s.commitMu.Unlock()
		s.commitRounds[e.Index] = blockCommit{round: e.Round, epoch: e.Epoch}
		if s.commitOldest == 0 {
			s.commitOldest = e.Index
		}
		for e.Index-s.commitOldest > commitRoundRetention {
			delete(s.commitRounds, s.commitOldest)
			s.commitOldest++
		}
		return nil
	})
	return s
}

// commitRoundFor returns the consensus round and committee epoch that
// committed the given block, if known.
func (s *Sequencer) commitRoundFor(block uint64) (blockCommit, bool) {
	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	c, ok := s.commitRounds[block]
	return c, ok
}

func (s *Sequencer) Type() api.ServiceType { return private.ServiceTypeSequencer }

func (s *Sequencer) Sequence(ctx context.Context, src, dst *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	if src == nil {
		return nil, errors.BadRequest.With("missing source")
	}
	if dst == nil {
		return nil, errors.BadRequest.With("missing destination")
	}
	if num == 0 {
		return nil, errors.BadRequest.With("missing sequence number")
	}
	if !s.partition.URL.ParentOf(src) {
		return nil, errors.BadRequest.WithFormat("requested source is %s but this partition is %s", src.RootIdentity(), s.partitionID)
	}

	globals := s.globals.Load().(*core.GlobalValues)
	if globals == nil {
		return nil, errors.NotReady
	}

	// Starting a batch would not be safe if the ABCI were updated to commit in
	// the middle of a block

	var r *api.MessageRecord[messaging.Message]
	var err error
	switch {
	case s.partition.Synthetic().Equal(src):
		return r, s.db.View(func(batch *database.Batch) error {
			r, err = s.getSynth(batch, globals, dst, num)
			return err
		})

	case s.partition.AnchorPool().Equal(src):
		return r, s.db.View(func(batch *database.Batch) error {
			r, err = s.getAnchor(batch, globals, dst, num)
			return err
		})
	}

	return nil, errors.BadRequest.WithFormat("invalid source: %s", src)
}

func (s *Sequencer) getAnchor(batch *database.Batch, globals *core.GlobalValues, dst *url.URL, num uint64) (*api.MessageRecord[messaging.Message], error) {
	chain, err := batch.Account(s.partition.AnchorPool()).AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}
	hash, err := chain.Entry(int64(num) - 1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", num-1, err)
	}

	var msg messaging.MessageWithTransaction
	err = batch.Message2(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = dst.JoinPath(protocol.AnchorPool)
	txn.Body = msg.GetTransaction().Body

	// If this is an anchor from the DN to the DN, we need to clear
	// MakeMajorBlock just like begin block does
	dirAnchor, ok := txn.Body.(*protocol.DirectoryAnchor)
	if ok && protocol.DnUrl().Equal(dst) {
		dirAnchor.MakeMajorBlock = 0
	}

	var signatures []protocol.Signature
	r := new(api.MessageRecord[messaging.Message])
	if globals.ExecutorVersion.V2Enabled() {
		r.Sequence = new(messaging.SequencedMessage)
		r.Sequence.Message = &messaging.TransactionMessage{Transaction: txn}
		r.Sequence.Source = s.partition.URL
		r.Sequence.Destination = dst
		r.Sequence.Number = num
		r.Message = &messaging.TransactionMessage{Transaction: txn}

		h := r.Sequence.Hash()
		hash = h[:]

	} else {
		// Create a partition signature
		partSig, err := new(signing.Builder).
			SetUrl(s.partition.URL).
			SetVersion(num).
			InitiateSynthetic(txn, dst)
		if err != nil {
			return nil, errors.InternalError.Wrap(err)
		}
		signatures = append(signatures, partSig)

		hash = txn.GetHash()
	}

	// Create a key signature
	signer := &protocol.UnknownSigner{Url: s.partition.JoinPath(protocol.Network)}
	keySig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(s.valKey).
		SetUrl(signer.Url).
		SetVersion(globals.Network.Version).
		SetTimestampToNow().
		Sign(hash)
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	signatures = append(signatures, keySig)

	sigSet := new(api.RecordRange[*api.MessageRecord[messaging.Message]])
	sigSet.Total = uint64(len(signatures))
	sigSet.Records = make([]*api.MessageRecord[messaging.Message], len(signatures))
	for i, sig := range signatures {
		sigSet.Records[i] = &api.MessageRecord[messaging.Message]{
			ID:      signer.Url.WithTxID(*(*[32]byte)(sig.Hash())),
			Message: &messaging.SignatureMessage{Signature: sig},
		}
	}

	r.ID = txn.ID()
	r.Message = &messaging.TransactionMessage{Transaction: txn}
	r.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
	r.Signatures.Total = 1
	r.Signatures.Records = []*api.SignatureSetRecord{{Signatures: sigSet}}
	return r, nil
}

func (s *Sequencer) getSynth(batch *database.Batch, globals *core.GlobalValues, dst *url.URL, num uint64) (*api.MessageRecord[messaging.Message], error) {
	// Load the appropriate sequence chain
	partition, ok := protocol.ParsePartitionUrl(dst)
	if !ok {
		return nil, errors.UnknownError.WithFormat("destination is not a partition")
	}
	ledger := batch.Account(s.partition.Synthetic())
	sequenceChain, err := ledger.SyntheticSequenceChain(partition).Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic sequence chain: %w", err)
	}

	// Load the Nth sequence chain entry
	sequenceEntry := new(protocol.IndexEntry)
	err = sequenceChain.EntryAs(int64(num)-1, sequenceEntry)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic sequence chain entry %d: %w", num-1, err)
	}

	// Load the corresponding main chain entry
	mainChain, err := ledger.MainChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic main chain: %w", err)
	}
	hash, err := mainChain.Entry(int64(sequenceEntry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic chain entry %d: %w", sequenceEntry.Source, err)
	}

	r := new(api.MessageRecord[messaging.Message])
	r.Signatures = new(api.RecordRange[*api.SignatureSetRecord])

	status, err := batch.Transaction(hash).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Load the transaction
	if globals.ExecutorVersion.V2Enabled() {
		var seq *messaging.SequencedMessage
		err = batch.Message2(hash).Main().GetAs(&seq)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}
		r.Sequence = seq
		r.Message = seq.Message

	} else {
		var msg messaging.MessageWithTransaction
		err = batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}
		hash = msg.GetTransaction().GetHash()
		r.Message = msg
		r.Sequence = new(messaging.SequencedMessage)
		r.Sequence.Message = msg
		r.Sequence.Source = status.SourceNetwork
		r.Sequence.Destination = status.DestinationNetwork
		r.Sequence.Number = status.SequenceNumber
		r.SourceReceipt = status.Proof
	}

	r.ID = r.Message.ID()

	// Sign the message
	keySig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(s.valKey).
		SetUrl(s.partition.JoinPath(protocol.Network)).
		SetVersion(globals.Network.Version).
		SetTimestampToNow().
		Sign(hash)
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}

	// Get the synthetic main chain receipt
	synthReceipt, mainAnchorEntry, err := s.getReceiptForChainEntry(ledger.MainChain(), sequenceEntry.Source)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	var receipt *merkle.Receipt
	continued, err := s.getRootContinuation(batch, mainAnchorEntry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if continued != nil {
		// Build the complete receipt
		receipt, err = synthReceipt.Combine(continued)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("combine receipts: %w", err)
		}
	}

	if globals.ExecutorVersion.V2Enabled() {
		r.SourceReceipt = receipt

		sigMsg := &messaging.SignatureMessage{
			Signature: keySig,
			TxID:      r.ID,
		}
		r.Signatures = &api.RecordRange[*api.SignatureSetRecord]{
			Total: 1,
			Records: []*api.SignatureSetRecord{{
				Account: &protocol.UnknownAccount{Url: keySig.GetSigner()},
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
					Total: 1,
					Records: []*api.MessageRecord[messaging.Message]{{
						ID:      sigMsg.ID(),
						Message: sigMsg,
					}},
				},
			}},
		}

	} else {
		var signatures []protocol.Signature

		// Add the partition signature
		partSig := new(protocol.PartitionSignature)
		partSig.SourceNetwork = status.SourceNetwork
		partSig.DestinationNetwork = status.DestinationNetwork
		partSig.SequenceNumber = status.SequenceNumber
		partSig.TransactionHash = *(*[32]byte)(hash)
		signatures = append(signatures, partSig)

		// Add the receipt signature
		receiptSig := new(protocol.ReceiptSignature)
		receiptSig.SourceNetwork = protocol.DnUrl()
		receiptSig.TransactionHash = *(*[32]byte)(hash)
		if receipt != nil {
			receiptSig.Proof = *receipt
		}
		signatures = append(signatures, receiptSig)

		// Add the key signature
		signatures = append(signatures, keySig)

		sigSet := new(api.RecordRange[*api.MessageRecord[messaging.Message]])
		sigSet.Total = uint64(len(signatures))
		sigSet.Records = make([]*api.MessageRecord[messaging.Message], len(signatures))
		for i, sig := range signatures {
			sigSet.Records[i] = &api.MessageRecord[messaging.Message]{
				ID:      keySig.GetSigner().WithTxID(*(*[32]byte)(sig.Hash())),
				Message: &messaging.SignatureMessage{Signature: sig},
			}
		}

		r.Signatures.Total = 1
		r.Signatures.Records = []*api.SignatureSetRecord{{Signatures: sigSet}}
	}

	r.Status = status.Code
	r.Error = status.Error
	r.Result = status.Result
	r.Received = status.Received
	return r, nil
}

// getRootContinuation builds a receipt from the synthetic main chain's root at
// the given index entry to a directory-anchored root. On the DN that is one
// hop through the root chain; on a BVN it continues through the directory's
// receipt for the block. Returns nil (without error) if this is a BVN and the
// directory has not yet receipted the block.
func (s *Sequencer) getRootContinuation(batch *database.Batch, mainAnchorEntry *protocol.IndexEntry) (*merkle.Receipt, error) {
	if strings.EqualFold(s.partitionID, protocol.Directory) {
		// We're on the DN, get the latest sent anchor
		anchorSequenceChain := batch.Account(s.partition.AnchorPool()).AnchorSequenceChain()
		head, err := anchorSequenceChain.Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor sequence chain head: %w", err)
		}
		if head.Count == 0 {
			return nil, errors.NotFound.With("anchor sequence chain is empty")
		}

		hash, err := anchorSequenceChain.Entry(head.Count - 1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", head.Count-1, err)
		}

		var msg messaging.MessageWithTransaction
		err = batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor #%d: %w", head.Count-1, err)
		}

		anchor, ok := msg.GetTransaction().Body.(*protocol.DirectoryAnchor)
		if !ok {
			return nil, errors.InternalError.WithFormat("invalid anchor sequence chain entry %d: want %v, got %v", head.Count-1, protocol.TransactionTypeDirectoryAnchor, msg.GetTransaction().Body.Type())
		}

		return s.getRootReceipt(batch, mainAnchorEntry.Anchor, anchor.RootChainIndex)
	}

	// We're on a BVN, get the latest directory anchor receipt
	dirReceipt, err := s.getDirectoryReceiptForBlock(batch, mainAnchorEntry.BlockIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if dirReceipt == nil {
		return nil, nil
	}

	// Get the receipt in between the other two
	rootReceipt, err := s.getRootReceipt(batch, mainAnchorEntry.Anchor, dirReceipt.Anchor.RootChainIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return rootReceipt.Combine(dirReceipt.RootChainReceipt)
}

// SequenceRange implements [private.SequenceRanger.SequenceRange]: it serves
// synthetic messages start through end (inclusive, 1-based) for the given
// destination, with a single collection proof (#4048) covering the whole
// range, set as SourceReceiptList on the last record.
func (s *Sequencer) SequenceRange(ctx context.Context, src, dst *url.URL, start, end uint64, _ private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	if src == nil {
		return nil, errors.BadRequest.With("missing source")
	}
	if dst == nil {
		return nil, errors.BadRequest.With("missing destination")
	}
	if start == 0 {
		return nil, errors.BadRequest.With("missing start")
	}
	if end < start {
		return nil, errors.BadRequest.WithFormat("invalid range [%d, %d]", start, end)
	}
	if end-start+1 > protocol.MaxReceiptListElements {
		return nil, errors.BadRequest.WithFormat("range [%d, %d] exceeds %d elements", start, end, protocol.MaxReceiptListElements)
	}
	if !s.partition.URL.ParentOf(src) {
		return nil, errors.BadRequest.WithFormat("requested source is %s but this partition is %s", src.RootIdentity(), s.partitionID)
	}

	globals := s.globals.Load().(*core.GlobalValues)
	if globals == nil {
		return nil, errors.NotReady
	}

	var r []*api.MessageRecord[messaging.Message]
	var err error
	switch {
	case s.partition.Synthetic().Equal(src):
		return r, s.db.View(func(batch *database.Batch) error {
			r, err = s.getSynthRange(batch, globals, dst, start, end)
			return err
		})
	case s.partition.AnchorPool().Equal(src):
		return r, s.db.View(func(batch *database.Batch) error {
			r, err = s.getAnchorRange(batch, globals, dst, start, end)
			return err
		})
	default:
		return nil, errors.BadRequest.With("only synthetic and anchor sequence ranges are supported")
	}
}

func (s *Sequencer) getSynthRange(batch *database.Batch, globals *core.GlobalValues, dst *url.URL, start, end uint64) ([]*api.MessageRecord[messaging.Message], error) {
	partition, ok := protocol.ParsePartitionUrl(dst)
	if !ok {
		return nil, errors.UnknownError.WithFormat("destination is not a partition")
	}
	ledger := batch.Account(s.partition.Synthetic())
	sequenceChain, err := ledger.SyntheticSequenceChain(partition).Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic sequence chain: %w", err)
	}

	mainChain, err := ledger.MainChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic main chain: %w", err)
	}

	// Map sequence numbers to synthetic main chain indices and load each
	// message. The main chain interleaves messages for all destinations, so
	// the proven range may cover more entries than the requested messages -
	// that is fine, extra elements are just proven hashes.
	indices := make([]uint64, 0, end-start+1)
	records := make([]*api.MessageRecord[messaging.Message], 0, end-start+1)
	for num := start; num <= end; num++ {
		sequenceEntry := new(protocol.IndexEntry)
		err = sequenceChain.EntryAs(int64(num)-1, sequenceEntry)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic sequence chain entry %d: %w", num-1, err)
		}
		indices = append(indices, sequenceEntry.Source)

		hash, err := mainChain.Entry(int64(sequenceEntry.Source))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic chain entry %d: %w", sequenceEntry.Source, err)
		}

		var seq *messaging.SequencedMessage
		err = batch.Message2(hash).Main().GetAs(&seq)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}

		// Sign the message
		keySig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(s.valKey).
			SetUrl(s.partition.JoinPath(protocol.Network)).
			SetVersion(globals.Network.Version).
			SetTimestampToNow().
			Sign(hash)
		if err != nil {
			return nil, errors.InternalError.Wrap(err)
		}

		r := new(api.MessageRecord[messaging.Message])
		r.ID = seq.Message.ID()
		r.Sequence = seq
		r.Message = seq.Message

		sigMsg := &messaging.SignatureMessage{
			Signature: keySig,
			TxID:      r.ID,
		}
		r.Signatures = &api.RecordRange[*api.SignatureSetRecord]{
			Total: 1,
			Records: []*api.SignatureSetRecord{{
				Account: &protocol.UnknownAccount{Url: keySig.GetSigner()},
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
					Total: 1,
					Records: []*api.MessageRecord[messaging.Message]{{
						ID:      sigMsg.ID(),
						Message: sigMsg,
					}},
				},
			}},
		}
		records = append(records, r)
	}

	// Extend the proven range to the block-boundary anchor point covering the
	// last message, so the proof can be continued to a directory-anchored root
	indexChain, err := ledger.MainChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic main index chain: %w", err)
	}
	if indexChain.Height() == 0 {
		return nil, errors.Conflict.With("synthetic main index chain is empty")
	}
	_, mainAnchorEntry, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height()-1), indexing.MatchAfter, indexing.SearchIndexChainBySource(indices[len(indices)-1]))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate index entry for synthetic chain entry %d: %w", indices[len(indices)-1], err)
	}

	continued, err := s.getRootContinuation(batch, mainAnchorEntry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if continued == nil {
		return nil, errors.NotReady.With("the directory has not receipted the block yet")
	}

	list, err := merkle.GetReceiptList(ledger.MainChain().Inner(), int64(indices[0]), int64(mainAnchorEntry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("build receipt list: %w", err)
	}
	list.ContinuedReceipt = continued
	if !list.Validate(nil) {
		return nil, errors.InternalError.With("built an invalid receipt list")
	}
	records[len(records)-1].SourceReceiptList = list

	return records, nil
}

// getAnchorRange serves anchors start through end (inclusive, 1-based) for
// the given destination, with a single collection proof covering the whole
// range (#4056). The proof is built over the anchor sequence chain, whose
// entries are the hashes of the anchor transactions in their canonical stored
// form (no principal). Unlike synthetic messages, anchor sequence numbers map
// directly to chain indices — sequence n is entry n-1 — and the same run
// serves every destination.
func (s *Sequencer) getAnchorRange(batch *database.Batch, globals *core.GlobalValues, dst *url.URL, start, end uint64) ([]*api.MessageRecord[messaging.Message], error) {
	ledger := batch.Account(s.partition.AnchorPool())
	sequenceChain, err := ledger.AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}
	if end > uint64(sequenceChain.Height()) {
		return nil, errors.NotFound.WithFormat("anchor %d not found: chain has %d entries", end, sequenceChain.Height())
	}

	records := make([]*api.MessageRecord[messaging.Message], 0, end-start+1)
	for num := start; num <= end; num++ {
		hash, err := sequenceChain.Entry(int64(num) - 1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", num-1, err)
		}

		var msg messaging.MessageWithTransaction
		err = batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor %d: %w", num, err)
		}

		// Serve the stored body verbatim — the proof binds the destination to
		// the canonical stored form, so the served body must hash to the
		// chain entry once the principal is stripped. Post-Vandenberg all
		// anchors sent from the DN are identical across destinations, and
		// this path is only used post-Kourou.
		txn := new(protocol.Transaction)
		txn.Header.Principal = dst.JoinPath(protocol.AnchorPool)
		txn.Body = msg.GetTransaction().Body

		seq := new(messaging.SequencedMessage)
		seq.Message = &messaging.TransactionMessage{Transaction: txn}
		seq.Source = s.partition.URL
		seq.Destination = dst
		seq.Number = num

		// Sign the sequenced message opportunistically. The collection proof
		// is the authorization; the signature only helps old-style checks.
		h := seq.Hash()
		keySig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(s.valKey).
			SetUrl(s.partition.JoinPath(protocol.Network)).
			SetVersion(globals.Network.Version).
			SetTimestampToNow().
			Sign(h[:])
		if err != nil {
			return nil, errors.InternalError.Wrap(err)
		}

		r := new(api.MessageRecord[messaging.Message])
		r.ID = txn.ID()
		r.Sequence = seq
		r.Message = seq.Message

		sigMsg := &messaging.SignatureMessage{
			Signature: keySig,
			TxID:      r.ID,
		}
		r.Signatures = &api.RecordRange[*api.SignatureSetRecord]{
			Total: 1,
			Records: []*api.SignatureSetRecord{{
				Account: &protocol.UnknownAccount{Url: keySig.GetSigner()},
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
					Total: 1,
					Records: []*api.MessageRecord[messaging.Message]{{
						ID:      sigMsg.ID(),
						Message: sigMsg,
					}},
				},
			}},
		}
		records = append(records, r)
	}

	// Extend the proven range to the block-boundary anchor point covering the
	// last entry, so the proof can be continued to a directory-anchored root
	indexChain, err := ledger.AnchorSequenceChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence index chain: %w", err)
	}
	if indexChain.Height() == 0 {
		return nil, errors.Conflict.With("anchor sequence index chain is empty")
	}
	_, anchorEntry, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height()-1), indexing.MatchAfter, indexing.SearchIndexChainBySource(end-1))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate index entry for anchor sequence chain entry %d: %w", end-1, err)
	}

	continued, err := s.getRootContinuation(batch, anchorEntry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if continued == nil {
		return nil, errors.NotReady.With("the directory has not receipted the block yet")
	}

	list, err := merkle.GetReceiptList(ledger.AnchorSequenceChain().Inner(), int64(start)-1, int64(anchorEntry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("build receipt list: %w", err)
	}
	list.ContinuedReceipt = continued
	if !list.Validate(nil) {
		return nil, errors.InternalError.With("built an invalid receipt list")
	}
	records[len(records)-1].SourceReceiptList = list

	return records, nil
}

// getReceiptForChainEntry gets a receipt from an entry to the first anchor
// after that entry.
func (s *Sequencer) getReceiptForChainEntry(chain *database.Chain2, index uint64) (*merkle.Receipt, *protocol.IndexEntry, error) {
	// Load the index chain
	indexChain, err := chain.Index().Get()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load %s index chain: %w", chain.Name(), err)
	}
	if indexChain.Height() == 0 {
		return nil, nil, errors.Conflict.WithFormat("%s index chain is empty", chain.Name())
	}

	// Locate the index entry for the given entry
	_, entry, err := indexing.SearchIndexChain(indexChain, uint64(indexChain.Height()-1), indexing.MatchAfter, indexing.SearchIndexChainBySource(index))
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("locate index entry for %s chain entry %d: %w", chain.Name(), index, err)
	}

	// Load the chain
	c, err := chain.Get()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load %s chain: %w", chain.Name(), err)
	}

	// Get a receipt
	receipt, err := c.Receipt(int64(index), int64(entry.Source))
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("get %s chain receipt from %d to %d: %w", chain.Name(), index, entry.Source, err)
	}

	return receipt, entry, nil
}

// getRootReceipt gets a root chain receipt.
func (s *Sequencer) getRootReceipt(batch *database.Batch, from, to uint64) (*merkle.Receipt, error) {
	// Load the root chain
	root, err := batch.Account(s.partition.Ledger()).RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}

	// Get a receipt from the entry to the block's anchor
	receipt, err := root.Receipt(int64(from), int64(to))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get root chain receipt from %d to %d: %w", from, to, err)
	}
	return receipt, nil
}

// getDirectoryReceiptForBlock returns the partition anchor receipt from the DN
// for the given block of this partition.
func (s *Sequencer) getDirectoryReceiptForBlock(batch *database.Batch, block uint64) (*protocol.PartitionAnchorReceipt, error) {
	// Find the anchor chain index entry for (or after) the block
	chain := batch.Account(s.partition.AnchorPool()).MainChain()
	head, err := chain.Index().Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor index chain head: %w", err)
	}
	if head.Count == 0 {
		return nil, errors.NotFound.With("anchor chain is empty")
	}
	_, entry, err := indexing.SearchIndexChain2(chain.Index(), uint64(head.Count-1), indexing.MatchAfter, indexing.SearchIndexChainByBlock(block))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate anchor index chain entry for block %d: %w", block, err)
	}

	// Find the DN anchor for the block
	head, err = chain.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor chain head: %w", err)
	}

	for i := int64(entry.Source); i < head.Count; i++ {
		entry, err := chain.Inner().Entry(i)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor chain entry %d: %w", i, err)
		}
		var msg messaging.MessageWithTransaction
		err = batch.Message2(entry).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor #%d: %w", i, err)
		}

		anchor, ok := msg.GetTransaction().Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}

		for _, r := range anchor.Receipts {
			if r.Anchor.Source.Equal(s.partition.URL) && r.Anchor.MinorBlockIndex >= block {
				return r, nil
			}
		}
	}
	return nil, nil
}
