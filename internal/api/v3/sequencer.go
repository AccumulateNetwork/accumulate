// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/cometbft/cometbft/libs/log"
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
}

var _ private.Sequencer = (*Sequencer)(nil)

type SequencerParams struct {
	Logger       log.Logger
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
	return s
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

		// Get the receipt in between the other two
		rootReceipt, err := s.getRootReceipt(batch, mainAnchorEntry.Anchor, anchor.RootChainIndex)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Build the complete receipt
		receipt, err = synthReceipt.Combine(rootReceipt)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("combine receipts: %w", err)
		}

	} else {
		// We're on a BVN, get the latest directory anchor receipt
		dirReceipt, err := s.getDirectoryReceiptForBlock(batch, mainAnchorEntry.BlockIndex)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		if dirReceipt != nil {
			// Get the receipt in between the other two
			rootReceipt, err := s.getRootReceipt(batch, mainAnchorEntry.Anchor, dirReceipt.Anchor.RootChainIndex)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}

			// Build the complete receipt
			receipt, err = synthReceipt.Combine(rootReceipt, dirReceipt.RootChainReceipt)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("combine receipts: %w", err)
			}
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
