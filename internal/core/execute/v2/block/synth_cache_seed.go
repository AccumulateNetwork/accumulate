// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// seedCacheBlocks is how many recent blocks the cache is rebuilt for at
// start: more than the Directory round trip, so every block whose anchor has
// not yet returned is held; far less than the horizon, so a start reads
// little. Older blocks are healing's to fill.
const seedCacheBlocks = 32

// seedSynthCache rebuilds the producer's cache for the recent blocks from the
// node's own chains, once, at the first block open (healing spec, "The
// cache"). Genesis produces its synthetics through another executor, and a
// node that starts has produced blocks whose anchors have not returned; both
// would otherwise miss at dispatch. This is the one place the cache is filled
// from the store, by position, and it is a start-up step, not a runtime path.
func (x *Executor) seedSynthCache(batch *database.Batch, current uint64) error {
	from := uint64(1)
	if current > seedCacheBlocks {
		from = current - seedCacheBlocks
	}
	var blocks []*synthcache.Block
	for b := from; b < current; b++ {
		blk, err := x.rebuildCacheBlock(batch, b)
		if err != nil {
			return errors.UnknownError.WithFormat("rebuild cache for block %d: %w", b, err)
		}
		blocks = append(blocks, blk)
	}
	x.synthCache().Seed(blocks)
	if len(blocks) > 0 {
		x.logger.Info("Seeded the synthetic cache from the chains", "module", "synthetic", "from", from, "to", current-1, "blocks", len(blocks))
	}
	return nil
}

// rebuildCacheBlock reads what block b produced and what its proofs are built
// from, by position. A block that produced nothing has no synthetic index
// entry and is held empty, so dispatch can tell "nothing to send" from a miss.
func (x *Executor) rebuildCacheBlock(batch *database.Batch, b uint64) (*synthcache.Block, error) {
	indexIndex, err := batch.SystemData(x.Describe.PartitionId).SyntheticIndexIndex(b).Get()
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return &synthcache.Block{Index: b}, nil
	default:
		return nil, errors.UnknownError.WithFormat("load synthetic index index: %w", err)
	}

	record := batch.Account(x.Describe.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic index chain: %w", err)
	}
	indexEntry := new(protocol.IndexEntry)
	err = synthIndexChain.EntryAs(int64(indexIndex), indexEntry)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic index chain entry %d: %w", indexIndex, err)
	}
	to := int64(indexEntry.Source)
	var from int64 = 1 // skip genesis
	if indexIndex > 0 {
		prev := new(protocol.IndexEntry)
		err = synthIndexChain.EntryAs(int64(indexIndex-1), prev)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic index chain entry %d: %w", indexIndex-1, err)
		}
		from = int64(prev.Source) + 1
	}

	synthChain, err := record.MainChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic chain: %w", err)
	}
	before, err := synthChain.State(from - 1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic chain state before %d: %w", from, err)
	}
	blk := &synthcache.Block{Index: b, Segment: &merkle.Segment{First: from, Before: before, MarkMask: record.MainChain().Inner().MarkMask()}}

	hashes, err := synthChain.Entries(from, to+1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load synthetic chain entries %d..%d: %w", from, to, err)
	}
	for i, hash := range hashes {
		var seq *messaging.SequencedMessage
		err := batch.Message2(hash).Main().GetAs(&seq)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic message: %w", err)
		}
		blk.Segment.Append(hash)
		e := &synthcache.Entry{Stream: seq.Destination, Number: seq.Number, Index: from + int64(i), Block: b, Hash: seq.Hash(), Seq: seq}
		if msg, ok := seq.Message.(messaging.MessageForTransaction); ok && seq.Message.Type() != messaging.MessageTypeBlockAnchor {
			var txn messaging.MessageWithTransaction
			err := batch.Message(msg.GetTxID().Hash()).Main().GetAs(&txn)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load transaction for synthetic message: %w", err)
			}
			e.Companion = txn
		}
		blk.Entries = append(blk.Entries, e)
	}

	// The receipt from the synthetic chain's anchor in the root chain to the
	// block's root
	ledger := batch.Account(x.Describe.Ledger())
	rootIndex, err := ledger.RootChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root index chain: %w", err)
	}
	if rootIndex.Height() == 0 {
		return nil, errors.NotFound.With("root index chain is empty")
	}
	_, rootEntry, err := indexing.SearchIndexChain(rootIndex, uint64(rootIndex.Height()-1), indexing.MatchExact, indexing.SearchIndexChainByBlock(b))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate block %d root index chain entry: %w", b, err)
	}
	root, err := ledger.RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}
	blk.RootReceipt, err = root.Receipt(int64(indexEntry.Anchor), int64(rootEntry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get root chain receipt from %d to %d: %w", indexEntry.Anchor, rootEntry.Source, err)
	}
	return blk, nil
}
