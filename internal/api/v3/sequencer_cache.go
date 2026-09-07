// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"bytes"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The answers below are built entirely from the producer's cache (healing
// spec, "The answer", "The cache"): no chain walk, no receipt from the store,
// no database read. An entry the cache does not hold is refused as NotFound
// and counted as a miss by the cache; a miss is a defect.

func (s *Sequencer) signRecord(globals *core.GlobalValues, hash []byte) (protocol.Signature, error) {
	return new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(s.valKey).
		SetUrl(s.partition.JoinPath(protocol.Network)).
		SetVersion(globals.Network.Version).
		SetTimestampToNow().
		Sign(hash)
}

func signatureSet(keySig protocol.Signature, id *url.TxID) *api.RecordRange[*api.SignatureSetRecord] {
	sigMsg := &messaging.SignatureMessage{Signature: keySig, TxID: id}
	return &api.RecordRange[*api.SignatureSetRecord]{
		Total: 1,
		Records: []*api.SignatureSetRecord{{
			Account: &protocol.UnknownAccount{Url: keySig.GetSigner()},
			Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
				Total:   1,
				Records: []*api.MessageRecord[messaging.Message]{{ID: sigMsg.ID(), Message: sigMsg}},
			},
		}},
	}
}

// entryRecord is the record for one cached synthetic entry, with its proof to
// the anchor its block was dispatched under, when it has been.
func (s *Sequencer) entryRecord(globals *core.GlobalValues, e *synthcache.Entry, blk *synthcache.Block) (*api.MessageRecord[messaging.Message], error) {
	r := new(api.MessageRecord[messaging.Message])
	r.Sequence = e.Seq
	r.Message = e.Seq.Message
	r.ID = r.Message.ID()
	r.Status = errors.Remote

	keySig, err := s.signRecord(globals, e.Hash[:])
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	r.Signatures = signatureSet(keySig, r.ID)
	r.Companion = e.Companion

	if blk != nil {
		r.SourceReceipt, err = blk.Proof(e.Stream, e.Index)
		if err != nil {
			return nil, errors.InternalError.WithFormat("build proof for %v: %w", r.ID, err)
		}
	}
	return r, nil
}

func (s *Sequencer) getSynthFromCache(globals *core.GlobalValues, dst *url.URL, num uint64) (*api.MessageRecord[messaging.Message], error) {
	e, ok := s.cache.Entry(dst, num)
	if !ok {
		return nil, errors.NotFound.WithFormat("synthetic %d for %v is not in the cache", num, dst)
	}
	blk, _ := s.cache.Block(e.Block)
	return s.entryRecord(globals, e, blk)
}

// getSynthRangeFromCache answers entries start..end for dst with one receipt
// list over their span of the synthetic chain, continued to the anchor the
// last entry's block was dispatched under. Consecutive blocks' segments join
// into one, so a range may span blocks.
// producedFor is how many synthetics this partition has produced for a
// destination, from its own synthetic ledger: mutable state, one read.
func (s *Sequencer) producedFor(dst *url.URL) (uint64, error) {
	var produced uint64
	err := s.db.View(func(batch *database.Batch) error {
		var ledger *protocol.SyntheticLedger
		err := batch.Account(s.partition.Synthetic()).Main().GetAs(&ledger)
		if err != nil {
			return err
		}
		produced = ledger.Partition(dst).Produced
		return nil
	})
	return produced, err
}

func (s *Sequencer) getSynthRangeFromCache(globals *core.GlobalValues, dst *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	var records []*api.MessageRecord[messaging.Message]
	var span *merkle.Segment
	var last *synthcache.Block
	var lastIndex uint64
	for num := start; num <= end; num++ {
		e, ok := s.cache.Peek(dst, num)
		if !ok {
			if num > start {
				break // the range ends where production has so far
			}
			// The first number asked for is not in the cache. Not produced
			// yet is "not yet", and no miss: a quiet stream is probed every
			// patience window and nothing is missing. Produced and gone is
			// a miss, and a defect.
			if produced, err := s.producedFor(dst); err == nil && num > produced {
				return nil, errors.NotReady.WithFormat("synthetic %d for %v is not produced yet (%d so far)", num, dst, produced)
			}
			synthcache.Count("entry", false)
			return nil, errors.NotFound.WithFormat("synthetic %d for %v is not in the cache", num, dst)
		}
		synthcache.Count("entry", true)
		if last == nil || last.Index != e.Block {
			blk, ok := s.cache.Block(e.Block)
			if !ok {
				return nil, errors.NotFound.WithFormat("block %d is not in the cache", e.Block)
			}
			// A block still in flight is not served: its entries are on
			// their way. Answer the dispatched prefix of the range, or "not
			// yet" when even the first block is in flight.
			if !s.cache.Servable(blk) {
				if len(records) > 0 {
					break
				}
				if !blk.Dispatched {
					return nil, errors.NotReady.With("the directory has not receipted the block yet")
				}
				return nil, errors.NotReady.WithFormat("block %d was dispatched %d blocks ago and is in flight", e.Block, s.cache.Newest()-blk.DispatchedAt)
			}
			st := blk.Stream(dst)
			if st == nil || st.Segment == nil {
				return nil, errors.NotFound.WithFormat("block %d holds no chain segment for %v", e.Block, dst)
			}
			switch {
			case span == nil:
				cp := *st.Segment
				span = &cp
			case st.Segment.First == span.Last()+1:
				span.Elements = append(append([][]byte(nil), span.Elements...), st.Segment.Elements...)
			default:
				return nil, errors.InternalError.WithFormat("blocks %d and %d are not consecutive on the synthetic chain", lastIndex, e.Block)
			}
			last, lastIndex = blk, e.Block
		}
		r, err := s.entryRecord(globals, e, nil)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	if len(records) == 0 {
		return nil, errors.BadRequest.With("empty range")
	}

	if opts.ProveAgainstAnchor > 0 && opts.ProveAgainstAnchor != last.AnchorBlock {
		return nil, errors.NotReady.WithFormat("the range is provable under Directory anchor %d, not %d", last.AnchorBlock, opts.ProveAgainstAnchor)
	}

	first, _ := s.cache.Entry(dst, start)
	list, err := span.ReceiptList(first.Index, span.Last())
	if err != nil {
		return nil, errors.InternalError.WithFormat("build receipt list: %w", err)
	}
	list.ContinuedReceipt, err = last.Continuation(dst)
	if err != nil {
		return nil, errors.InternalError.WithFormat("continue receipt list: %w", err)
	}
	if !list.Validate(nil) {
		return nil, errors.InternalError.With("built an invalid receipt list")
	}
	records[len(records)-1].SourceReceiptList = list
	records[len(records)-1].SourceAnchorBlock = last.AnchorBlock
	return records, nil
}

// anchorRecord shapes a produced anchor for dst as the store path does: the
// principal is the destination's anchor pool, and the Directory does not ask
// itself to open a major block.
func (s *Sequencer) anchorRecord(globals *core.GlobalValues, dst *url.URL, num uint64, produced *protocol.Transaction) (*api.MessageRecord[messaging.Message], error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = dst.JoinPath(protocol.AnchorPool)
	txn.Body = produced.Body.CopyAsInterface().(protocol.TransactionBody)
	if da, ok := txn.Body.(*protocol.DirectoryAnchor); ok && protocol.DnUrl().Equal(dst) {
		da.MakeMajorBlock = 0
	}

	r := new(api.MessageRecord[messaging.Message])
	r.Sequence = &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      s.partition.URL,
		Destination: dst,
		Number:      num,
	}
	r.Message = r.Sequence.Message
	r.ID = txn.ID()
	r.Status = errors.Remote

	h := r.Sequence.Hash()
	keySig, err := s.signRecord(globals, h[:])
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	r.Signatures = signatureSet(keySig, r.ID)

	// The quorum the source already holds. A partition delivers its own
	// anchor to itself with every validator's signature; the destination
	// accepts a signature made over the source's own copy on the copy sent
	// to it (BlockAnchor.checkSignature, signature reuse). One answer then
	// carries the quorum, instead of one signature from whichever node
	// answered — copies lost in dispatch are not re-sent (run
	// 20260905T225751Z: fewer than the threshold arrived, nothing re-sent,
	// every stream waited on the anchor).
	own := new(protocol.Transaction)
	own.Header.Principal = s.partition.URL.JoinPath(protocol.AnchorPool)
	own.Body = produced.Body
	var held []protocol.KeySignature
	err = s.db.View(func(batch *database.Batch) error {
		var err error
		held, err = batch.Account(own.Header.Principal).Transaction(own.ID().Hash()).ValidatorSignatures().Get()
		return err
	})
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.UnknownError.WithFormat("load held anchor signatures: %w", err)
	}
	ownKey, _ := keySig.(protocol.KeySignature)
	for _, sig := range held {
		if ownKey != nil && bytes.Equal(sig.GetPublicKey(), ownKey.GetPublicKey()) {
			continue
		}
		set := signatureSet(sig, r.ID)
		r.Signatures.Records = append(r.Signatures.Records, set.Records...)
		r.Signatures.Total++
	}
	return r, nil
}

func (s *Sequencer) getAnchorFromCache(globals *core.GlobalValues, dst *url.URL, num uint64) (*api.MessageRecord[messaging.Message], error) {
	txn, _, ok := s.cache.Anchor(num)
	if !ok {
		return nil, errors.NotFound.WithFormat("anchor %d is not in the cache", num)
	}
	return s.anchorRecord(globals, dst, num, txn)
}

// getAnchorRangeFromCache answers anchors start..end, or the prefix of them
// the cache holds. An anchor is validated at the destination by its
// signatures — the answering validator's counts as one — so no receipt list
// travels with a range of them. A first number not produced yet, or recorded
// within the in-flight window, is NotReady: it is on its way, not missing
// (healing spec, "The answer").
func (s *Sequencer) getAnchorRangeFromCache(globals *core.GlobalValues, dst *url.URL, start, end uint64) ([]*api.MessageRecord[messaging.Message], error) {
	var records []*api.MessageRecord[messaging.Message]
	for num := start; num <= end; num++ {
		txn, block, ok := s.cache.PeekAnchor(num)
		if !ok {
			if len(records) > 0 {
				return records, nil
			}
			if last, ok := s.cache.LastAnchorNumber(); !ok || num > last {
				return nil, errors.NotReady.WithFormat("anchor %d is not produced yet", num)
			}
			synthcache.Count("anchor", false)
			return nil, errors.NotFound.WithFormat("anchor %d is not in the cache", num)
		}
		synthcache.Count("anchor", true)
		if age := s.cache.Newest() - block; age < synthcache.InFlightBlocks {
			if len(records) > 0 {
				return records, nil
			}
			return nil, errors.NotReady.WithFormat("anchor %d was sent %d blocks ago and is in flight", num, age)
		}
		r, err := s.anchorRecord(globals, dst, num, txn)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
