// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var _ private.MajorHeaderRanger = (*Sequencer)(nil)

// MajorHeaderRange implements [private.MajorHeaderRanger.MajorHeaderRange]: it
// serves major blocks start through end (inclusive, 1-based), each with the
// partition's self-anchor for the minor block that closed the major block and
// the archived validator-quorum signatures over that anchor (#4058). Only the
// directory serves this — it is the only partition that anchors to itself, so
// it is the only partition whose own database archives the quorum for its own
// anchors.
func (s *Sequencer) MajorHeaderRange(ctx context.Context, partition *url.URL, start, end uint64, _ private.SequenceOptions) ([]*private.MajorHeaderRecord, error) {
	if partition == nil {
		return nil, errors.BadRequest.With("missing partition")
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
	if !s.partition.URL.Equal(partition) {
		return nil, errors.BadRequest.WithFormat("requested partition is %s but this partition is %s", partition, s.partitionID)
	}
	if !protocol.DnUrl().Equal(partition) {
		return nil, errors.BadRequest.With("major headers are only served for the directory")
	}

	var r []*private.MajorHeaderRecord
	var err error
	return r, s.db.View(func(batch *database.Batch) error {
		r, err = s.getMajorHeaderRange(batch, start, end)
		return err
	})
}

func (s *Sequencer) getMajorHeaderRange(batch *database.Batch, start, end uint64) ([]*private.MajorHeaderRecord, error) {
	account := batch.Account(s.partition.AnchorPool())
	majorChain, err := account.MajorBlockChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load major block chain: %w", err)
	}
	if majorChain.Height() == 0 {
		return nil, errors.NotFound.With("no major blocks exist")
	}

	// Clamp to the last major block so clients can page blindly — a short
	// page tells the client it has reached the end of the spine
	last := new(protocol.IndexEntry)
	err = majorChain.EntryAs(majorChain.Height()-1, last)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load last major block entry: %w", err)
	}
	if start > last.BlockIndex {
		return nil, errors.NotFound.WithFormat("major block %d not found: the last major block is %d", start, last.BlockIndex)
	}
	if end > last.BlockIndex {
		end = last.BlockIndex
	}

	seqChain, err := account.AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}

	records := make([]*private.MajorHeaderRecord, 0, end-start+1)
	for index := start; index <= end; index++ {
		entryIndex, entry, err := indexing.SearchIndexChain(majorChain, index-1, indexing.MatchExact, indexing.SearchIndexChainByBlock(index))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get major block %d: %w", index, err)
		}

		// Resolve the minor block that closed the major block
		rootEntry, rootPrev, err := getMajorBlockBounds(s.partition, batch, entry, entryIndex)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		anchor, num, err := s.findSelfAnchorForBlock(batch, seqChain, rootEntry.BlockIndex)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("locate closing anchor for major block %d (minor block %d): %w", index, rootEntry.BlockIndex, err)
		}

		updates, err := s.getNetworkUpdatesInWindow(batch, rootPrev.BlockIndex, anchor.GetPartitionAnchor())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect network updates for major block %d: %w", index, err)
		}

		seq, sigs, err := s.buildSelfAnchor(batch, anchor, num)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("build self-anchor for major block %d: %w", index, err)
		}

		records = append(records, &private.MajorHeaderRecord{
			Index:      index,
			Entry:      entry,
			Anchor:     seq,
			Signatures: sigs,
			Updates:    updates,
		})
	}
	return records, nil
}

// getNetworkUpdatesInWindow collects the network-account transactions
// executed after the previous major block's close and at or before the given
// closing anchor's block, each with a receipt proving it into the anchor's
// root chain anchor. These carry the validator-set timeline: a spine walker
// applies them without per-anchor quorum checks because the receipts bind
// them to a quorum-verified root.
func (s *Sequencer) getNetworkUpdatesInWindow(batch *database.Batch, prevClose uint64, close *protocol.PartitionAnchor) ([]*private.NetworkUpdateProof, error) {
	var updates []*private.NetworkUpdateProof
	for _, path := range []string{protocol.Network, protocol.Globals} {
		account := batch.Account(s.partition.JoinPath(path))
		chain2 := account.MainChain()
		chain, err := chain2.Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %s main chain: %w", path, err)
		}

		for i := int64(0); i < chain.Height(); i++ {
			entryReceipt, indexEntry, err := s.getReceiptForChainEntry(chain2, uint64(i))
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			if indexEntry.BlockIndex <= prevClose || indexEntry.BlockIndex > close.MinorBlockIndex {
				continue
			}

			rootReceipt, err := s.getRootReceipt(batch, indexEntry.Anchor, close.RootChainIndex)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			receipt, err := entryReceipt.Combine(rootReceipt)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("combine %s update receipt: %w", path, err)
			}

			hash, err := chain.Entry(i)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load %s main chain entry %d: %w", path, i, err)
			}
			var msg messaging.MessageWithTransaction
			err = batch.Message2(hash).Main().GetAs(&msg)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load %s update transaction %x: %w", path, hash, err)
			}

			updates = append(updates, &private.NetworkUpdateProof{
				Transaction: msg.GetTransaction(),
				Receipt:     receipt,
			})
		}
	}
	return updates, nil
}

// buildSelfAnchor reconstructs the partition's self-anchor as it was
// received and loads its archived validator-quorum signatures. Begin block
// clears MakeMajorBlock on anchors from the DN to the DN, so the executed
// transaction — the one the signatures are archived under — has it zeroed
// (see Sequencer.getAnchor).
func (s *Sequencer) buildSelfAnchor(batch *database.Batch, anchor protocol.AnchorBody, num uint64) (*messaging.SequencedMessage, []protocol.KeySignature, error) {
	if dir, ok := anchor.(*protocol.DirectoryAnchor); ok {
		dir.MakeMajorBlock = 0
	}
	txn := new(protocol.Transaction)
	txn.Header.Principal = s.partition.AnchorPool()
	txn.Body = anchor

	seq := new(messaging.SequencedMessage)
	seq.Message = &messaging.TransactionMessage{Transaction: txn}
	seq.Source = s.partition.URL
	seq.Destination = s.partition.URL
	seq.Number = num

	sigs, err := batch.Account(s.partition.AnchorPool()).Transaction(txn.ID().Hash()).ValidatorSignatures().Get()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load validator signatures: %w", err)
	}
	return seq, sigs, nil
}

// findSelfAnchorForBlock finds the first anchor in the sequence chain whose
// MinorBlockIndex is at least the given block, and returns a copy of its body
// and its sequence number. MinorBlockIndex increases monotonically along the
// sequence chain, so a binary search suffices.
func (s *Sequencer) findSelfAnchorForBlock(batch *database.Batch, seqChain *database.Chain, block uint64) (protocol.AnchorBody, uint64, error) {
	height := uint64(seqChain.Height())
	var searchErr error
	i := sort.Search(int(height), func(i int) bool {
		if searchErr != nil {
			return false
		}
		body, err := s.loadAnchorBody(batch, seqChain, uint64(i)+1)
		if err != nil {
			searchErr = err
			return false
		}
		return body.GetPartitionAnchor().MinorBlockIndex >= block
	})
	if searchErr != nil {
		return nil, 0, searchErr
	}
	if uint64(i) >= height {
		return nil, 0, errors.NotFound.WithFormat("no anchor at or after minor block %d", block)
	}

	num := uint64(i) + 1
	body, err := s.loadAnchorBody(batch, seqChain, num)
	if err != nil {
		return nil, 0, err
	}
	return body.CopyAsInterface().(protocol.AnchorBody), num, nil
}
