// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var _ private.PartitionRootRanger = (*Sequencer)(nil)

// PartitionRootRange implements
// [private.PartitionRootRanger.PartitionRootRange] (#4058 phase 3b). The
// directory records every received partition anchor's StateTreeAnchor on
// AnchorChain(partition).BPT(), so proving a BVN's state root needs no BVN
// validator quorum: one receipt from that entry to the root chain anchor of a
// quorum-anchored directory block, which the client binds its spine walk to.
func (s *Sequencer) PartitionRootRange(ctx context.Context, partition *url.URL, stateRoot [32]byte, _ private.SequenceOptions) (*private.PartitionRootRecord, error) {
	if partition == nil {
		return nil, errors.BadRequest.With("missing partition")
	}
	if !protocol.DnUrl().Equal(s.partition.URL) {
		return nil, errors.BadRequest.With("partition roots are only served by the directory")
	}
	id, ok := protocol.ParsePartitionUrl(partition)
	if !ok {
		return nil, errors.BadRequest.WithFormat("%v is not a partition URL", partition)
	}

	var r *private.PartitionRootRecord
	var err error
	return r, s.db.View(func(batch *database.Batch) error {
		r, err = s.getPartitionRootRange(batch, id, stateRoot)
		return err
	})
}

func (s *Sequencer) getPartitionRootRange(batch *database.Batch, partition string, stateRoot [32]byte) (*private.PartitionRootRecord, error) {
	// Locate the state root on the directory's record of the partition's
	// anchors. Entries are only ever appended by executing the partition's
	// (validated, in-sequence) anchors, so membership proves the root is a
	// genuine anchored state of that partition.
	bptChain, err := batch.Account(s.partition.AnchorPool()).AnchorChain(partition).BPT().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load %s BPT anchor chain: %w", partition, err)
	}
	entry, err := bptChain.HeightOf(stateRoot[:])
	switch {
	case err == nil:
		// found
	case errors.Code(err) == errors.NotFound:
		return nil, errors.NotFound.WithFormat("state root %x is not recorded for %s — its anchor may not have been delivered yet", stateRoot[:8], partition)
	default:
		return nil, errors.UnknownError.WithFormat("locate state root on %s BPT anchor chain: %w", partition, err)
	}

	// Receipt from the entry to the chain anchor recorded when its block was
	// folded into the directory's root chain
	entryReceipt, indexEntry, err := s.getReceiptForChainEntry(batch.Account(s.partition.AnchorPool()).AnchorChain(partition).BPT(), uint64(entry))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// The receipt must end at a quorum-anchored directory block so the client
	// can bind its spine walk to it. The newest self-anchors have not
	// accumulated their quorum yet — target the newest one that has.
	seqChain, err := batch.Account(s.partition.AnchorPool()).AnchorSequenceChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}
	_, endBody, err := s.findLatestQuorumAnchor(batch, seqChain, 0)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	end := endBody.GetPartitionAnchor()
	if end.RootChainIndex < indexEntry.Anchor {
		return nil, errors.NotReady.WithFormat("the state root was recorded after directory block %d, the newest with a quorum", end.MinorBlockIndex)
	}

	// Extend the receipt through the root chain to the target anchor's root
	rootReceipt, err := s.getRootReceipt(batch, indexEntry.Anchor, end.RootChainIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	receipt, err := entryReceipt.Combine(rootReceipt)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("combine receipts: %w", err)
	}

	return &private.PartitionRootRecord{
		Receipt:        receipt,
		DirectoryBlock: end.MinorBlockIndex,
	}, nil
}
