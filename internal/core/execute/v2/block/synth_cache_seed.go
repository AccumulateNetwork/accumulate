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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ownReceipt is a Directory receipt for one of this partition's blocks,
// carried by a Directory anchor this partition executed.
type ownReceipt struct {
	block       uint64                           // the own block receipted
	anchorBlock uint64                           // the Directory block whose anchor carried it
	receipt     *protocol.PartitionAnchorReceipt // nil on the Directory, whose own anchor receipts its own block
}

// seedSynthCache rebuilds the producer's cache at start, once, at the first
// block open (healing spec, "The cache"). What it holds is decided by what the
// store durably knows (#4241):
//
//   - Every own block the Directory has not receipted yet. Nothing from it
//     has been dispatched, so every entry it produced is still to send; a
//     restart with a lagging Directory holds them all, up to the horizon.
//   - The blocks in flight below the newest receipt: dispatched, and what a
//     destination may still be executing or asking healing for. The
//     destination's own Delivered of this stream is not durable here, so
//     this is the bound (DIFFERENCES, H1).
//
// The receipts come from the Directory anchors executed here, the same
// anchors whose dispatch the block after them performs. That list is memory
// and a restart loses it, so what the newest anchor receipted is marked
// dispatched again — healing serves only dispatched blocks — and sent again
// by the leader. A destination tosses what it has delivered, so the resend
// costs a package, never a duplicate execution.
//
// This is the one place the cache is filled from the store, by position, and
// it is a start-up step, not a runtime path.
func (x *Executor) seedSynthCache(batch *database.Batch, current uint64, isLeader bool) error {
	oldest := uint64(1)
	if current > synthcache.DefaultHorizon {
		oldest = current - synthcache.DefaultHorizon
	}

	// What the Directory has receipted of ours, newest first
	receipts, err := x.ownReceipts(batch, oldest)
	if err != nil {
		return errors.UnknownError.WithFormat("load Directory receipts: %w", err)
	}
	from := oldest
	if len(receipts) > 0 {
		newest := receipts[0].block
		if newest > synthcache.InFlightBlocks && newest-synthcache.InFlightBlocks > from {
			from = newest - synthcache.InFlightBlocks
		}
	}

	// The blocks a join carried this node past -- after the block its
	// executor last executed, through the block the join settled at -- it did
	// not execute: it produced none of their synthetics and holds none of
	// their messages, since the join pulls <partition>/synthetic state-only.
	// They are not its to rebuild, dispatch or serve (executor.md, "Sync" §6).
	// Rebuilding them read messages the node never had and failed the first
	// block it opened; skipping every block at or below the join instead left
	// a restart that fell nothing behind with an empty cache (#4400).
	var blocks []*synthcache.Block
	for b := from; b < current; b++ {
		if x.synthCache().NotExecuted(b) {
			continue
		}
		blk, err := x.rebuildCacheBlock(batch, b)
		if err != nil {
			return errors.UnknownError.WithFormat("rebuild cache for block %d: %w", b, err)
		}
		blocks = append(blocks, blk)
	}
	x.synthCache().Seed(blocks)
	if len(blocks) > 0 {
		x.logger.Info("Seeded the synthetic cache from the chains", "module", "synthetic", "from", from, "to", current-1, "blocks", len(blocks), "receipted", len(receipts))
	}
	err = x.seedProducedAnchors(batch, oldest)
	if err != nil {
		return errors.UnknownError.WithFormat("seed produced anchors: %w", err)
	}

	if len(receipts) == 0 {
		return nil
	}

	// The receipted blocks are dispatched; the newest anchor's are dispatched
	// again, in case the block that would have done it never ran
	var ledger *protocol.SyntheticLedger
	err = batch.Account(x.Describe.Synthetic()).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}
	deliveredFrom := func(dst *url.URL) uint64 { return ledger.Partition(dst).Delivered }
	for _, r := range receipts {
		if r.block < from || x.synthCache().NotExecuted(r.block) {
			continue
		}
		if r.anchorBlock == receipts[0].anchorBlock {
			err = x.sendSyntheticTransactionsForBlock(r.block, r.receipt, r.anchorBlock, isLeader, deliveredFrom)
			if err != nil {
				return errors.UnknownError.WithFormat("dispatch block %d: %w", r.block, err)
			}
			continue
		}
		x.synthCache().MarkDispatched(r.block, r.anchorBlock, r.receipt)
	}
	return nil
}

// ownReceipts walks the Directory anchors this partition executed, newest
// first, and returns their receipts for this partition's blocks down to the
// in-flight tail below the newest, or to oldest.
func (x *Executor) ownReceipts(batch *database.Batch, oldest uint64) ([]ownReceipt, error) {
	c := batch.Account(x.Describe.AnchorPool()).MainChain()
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor pool main chain head: %w", err)
	}
	own := x.Describe.PartitionUrl().URL
	var receipts []ownReceipt
	bound := oldest
	for i := head.Count - 1; i >= 0; i-- {
		entry, err := c.Entry(i)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor pool main chain entry %d: %w", i, err)
		}
		var msg *messaging.TransactionMessage
		err = batch.Message2(entry).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor pool main chain entry %d: %w", i, err)
		}
		body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			continue
		}
		var found []ownReceipt
		if x.Describe.NetworkType == protocol.PartitionTypeDirectory {
			found = append(found, ownReceipt{block: body.MinorBlockIndex, anchorBlock: body.MinorBlockIndex})
		}
		for _, r := range body.Receipts {
			if own.LocalTo(r.Anchor.Source) {
				found = append(found, ownReceipt{block: r.Anchor.MinorBlockIndex, anchorBlock: body.MinorBlockIndex, receipt: r})
			}
		}
		if len(found) == 0 {
			continue
		}
		if len(receipts) == 0 {
			// The newest receipt sets the tail
			newest := found[0].block
			for _, r := range found {
				if r.block > newest {
					newest = r.block
				}
			}
			if newest > synthcache.InFlightBlocks && newest-synthcache.InFlightBlocks > bound {
				bound = newest - synthcache.InFlightBlocks
			}
		}
		receipts = append(receipts, found...)
		below := true
		for _, r := range found {
			if r.block >= bound {
				below = false
			}
		}
		if below {
			break
		}
	}
	return receipts, nil
}

// rebuildCacheBlock reads what block b produced and what its proofs are built
// from, by position, one destination chain at a time (executor spec, "One
// chain per pair, one stage per chain"). A block that appended to no chain is
// held empty, so dispatch can tell "nothing to send" from a miss.
func (x *Executor) rebuildCacheBlock(batch *database.Batch, b uint64) (*synthcache.Block, error) {
	blk := &synthcache.Block{Index: b, Streams: map[string]*synthcache.Stream{}}

	record := batch.Account(x.Describe.Synthetic())
	for _, part := range x.globals().Active.Network.Partitions {
		dst := protocol.PartitionUrl(part.ID)
		c := record.SyntheticChain(part.ID)
		// Existence is read from the head: Chain2.Get registers the chain on
		// the account, and a seed must write nothing. A chain never appended
		// to has no head and is skipped before anything registers it.
		head, err := c.Index().Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic index chain head for %v: %w", dst, err)
		}
		if head.Count == 0 {
			continue
		}
		index, err := c.Index().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic index chain for %v: %w", dst, err)
		}
		indexIndex, indexEntry, err := indexing.SearchIndexChain(index, uint64(index.Height()-1), indexing.MatchExact, indexing.SearchIndexChainByBlock(b))
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue // nothing for this destination in block b
		default:
			return nil, errors.UnknownError.WithFormat("locate block %d on the synthetic index chain for %v: %w", b, dst, err)
		}
		to := int64(indexEntry.Source)
		var from int64
		if indexIndex > 0 {
			prev := new(protocol.IndexEntry)
			err = index.EntryAs(int64(indexIndex-1), prev)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load synthetic index chain entry %d for %v: %w", indexIndex-1, dst, err)
			}
			from = int64(prev.Source) + 1
		}

		chain, err := c.Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic chain for %v: %w", dst, err)
		}
		before, err := chain.State(from - 1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic chain state for %v before %d: %w", dst, from, err)
		}
		st := &synthcache.Stream{Destination: dst, ChainName: c.Name(), IndexIndex: indexIndex, RootPos: int64(indexEntry.Anchor), Segment: &merkle.Segment{First: from, Before: before, MarkMask: c.Inner().MarkMask()}}
		blk.Streams[synthcache.StreamKey(dst)] = st

		hashes, err := chain.Entries(from, to+1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic chain entries %d..%d for %v: %w", from, to, dst, err)
		}
		for i, hash := range hashes {
			var seq *messaging.SequencedMessage
			err := batch.Message2(hash).Main().GetAs(&seq)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load synthetic message: %w", err)
			}
			st.Segment.Append(hash)
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
	}
	if len(blk.Streams) == 0 {
		return blk, nil
	}

	// The block's root chain entry: every stream's root receipt runs from the
	// stream's anchor to it.
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
	for _, st := range blk.Streams {
		st.RootReceipt, err = root.Receipt(st.RootPos, int64(rootEntry.Source))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get root chain receipt from %d to %d: %w", st.RootPos, rootEntry.Source, err)
		}
	}
	return blk, nil
}

// seedProducedAnchors puts this partition's own anchors back in the cache at
// start, down to the horizon (#4277).
//
// The anchor sequence chain is the durable record of what this partition
// produced: entry i is the anchor with sequence number i+1. The cache that
// answers for them is memory, filled a block at a time as each anchor is
// produced, so without this a restart can serve nothing it produced before —
// and a destination that is behind on the stream asks, is refused, and stays
// behind, because there is no other place the anchor can come from. The
// heartbeat makes that reachable: it produces enough anchors that a
// destination is routinely behind by more than a restart can re-send.
func (x *Executor) seedProducedAnchors(batch *database.Batch, oldest uint64) error {
	record := batch.Account(x.Describe.AnchorPool()).AnchorSequenceChain()
	head, err := record.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor sequence chain head: %w", err)
	}
	if head.Count == 0 {
		return nil
	}
	chain, err := record.Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
	}

	// Newest first, and stop at the horizon: an anchor for a block the cache
	// no longer covers is not one this partition answers for.
	var anchors []synthcache.SeededAnchor
	for i := head.Count - 1; i >= 0; i-- {
		hash, err := chain.Entry(i)
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", i, err)
		}
		var txn *messaging.TransactionMessage
		err = batch.Message2(hash).Main().GetAs(&txn)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue // pruned; nothing to answer with
		default:
			return errors.UnknownError.WithFormat("load anchor %d: %w", i+1, err)
		}
		body, ok := txn.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			continue
		}
		block := body.GetPartitionAnchor().MinorBlockIndex
		if block < oldest {
			break
		}
		anchors = append(anchors, synthcache.SeededAnchor{Number: uint64(i) + 1, Block: block, Txn: txn.Transaction})
	}
	if len(anchors) == 0 {
		return nil
	}
	x.synthCache().SeedAnchors(anchors)
	x.logger.Info("Seeded produced anchors from the sequence chain", "module", "synthetic",
		"count", len(anchors), "from", anchors[len(anchors)-1].Number, "to", anchors[0].Number)
	return nil
}

// seedCacheOnce seeds the cache unless a seed has already succeeded. A seed
// that fails is not counted, so the next block to open tries again rather than
// running on a cache nothing filled (#4400).
func (x *Executor) seedCacheOnce(batch *database.Batch, current uint64, isLeader bool) error {
	x.cacheSeedMu.Lock()
	defer x.cacheSeedMu.Unlock()
	if x.cacheSeeded {
		return nil
	}
	if err := x.seedSynthCache(batch, current, isLeader); err != nil {
		return err
	}
	x.cacheSeeded = true
	return nil
}
