// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
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
		r.SourceReceipt, err = blk.Proof(e.Index)
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
func (s *Sequencer) getSynthRangeFromCache(globals *core.GlobalValues, dst *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	var records []*api.MessageRecord[messaging.Message]
	var span *merkle.Segment
	var last *synthcache.Block
	var lastIndex uint64
	for num := start; num <= end; num++ {
		e, ok := s.cache.Entry(dst, num)
		if !ok {
			return nil, errors.NotFound.WithFormat("synthetic %d for %v is not in the cache", num, dst)
		}
		if last == nil || last.Index != e.Block {
			blk, ok := s.cache.Block(e.Block)
			if !ok {
				return nil, errors.NotFound.WithFormat("block %d is not in the cache", e.Block)
			}
			switch {
			case span == nil:
				cp := *blk.Segment
				span = &cp
			case blk.Segment.First == span.Last()+1:
				span.Elements = append(append([][]byte(nil), span.Elements...), blk.Segment.Elements...)
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

	if !last.Dispatched {
		return nil, errors.NotReady.With("the directory has not receipted the block yet")
	}
	if opts.ProveAgainstAnchor > 0 && opts.ProveAgainstAnchor != last.AnchorBlock {
		return nil, errors.NotReady.WithFormat("the range is provable under Directory anchor %d, not %d", last.AnchorBlock, opts.ProveAgainstAnchor)
	}

	first, _ := s.cache.Entry(dst, start)
	list, err := span.ReceiptList(first.Index, span.Last())
	if err != nil {
		return nil, errors.InternalError.WithFormat("build receipt list: %w", err)
	}
	list.ContinuedReceipt, err = last.Continuation()
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
	return r, nil
}

func (s *Sequencer) getAnchorFromCache(globals *core.GlobalValues, dst *url.URL, num uint64) (*api.MessageRecord[messaging.Message], error) {
	txn, ok := s.cache.Anchor(num)
	if !ok {
		return nil, errors.NotFound.WithFormat("anchor %d is not in the cache", num)
	}
	return s.anchorRecord(globals, dst, num, txn)
}

// getAnchorRangeFromCache answers anchors start..end. An anchor is validated
// by its signatures at the destination, and a later anchor's chain proves an
// earlier one, so no receipt list travels with a range of them.
func (s *Sequencer) getAnchorRangeFromCache(globals *core.GlobalValues, dst *url.URL, start, end uint64) ([]*api.MessageRecord[messaging.Message], error) {
	var records []*api.MessageRecord[messaging.Message]
	for num := start; num <= end; num++ {
		txn, ok := s.cache.Anchor(num)
		if !ok {
			return nil, errors.NotFound.WithFormat("anchor %d is not in the cache", num)
		}
		r, err := s.anchorRecord(globals, dst, num, txn)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
