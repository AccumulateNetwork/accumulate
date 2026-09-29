// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	stderrors "errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
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
// store durably knows (#4241, #4464):
//
//   - Every entry a destination has not delivered: each stream from the
//     number after the destination's Delivered, as this node recorded it
//     when its cache applied the destination's word (SystemData
//     SyntheticDelivered). A whole-network restart has no peer that holds
//     them, and the cache holds an entry until it is delivered, so this is
//     what the running cache held.
//   - Every own block the Directory has not receipted yet, and the blocks in
//     flight below the newest receipt, as before: dispatch looks each of them
//     up, an empty one included.
//
// The receipts come from the Directory anchors executed here, the same
// anchors whose dispatch the block after them performs. That list is memory
// and a restart loses it, so what the newest anchor receipted is marked
// dispatched again — healing serves only dispatched blocks — and sent again
// by the leader; the older receipted blocks are marked dispatched under the
// anchor that receipted them, back to the oldest undelivered entry. A
// destination tosses what it has delivered, so the resend costs a package,
// never a duplicate execution.
//
// This is the one place the cache is filled from the store, by position, and
// it is a start-up step, not a runtime path.
func (x *Executor) seedSynthCache(batch *database.Batch, current uint64, isLeader bool) error {
	oldest := uint64(1)
	if current > synthcache.DefaultHorizon {
		oldest = current - synthcache.DefaultHorizon
	}

	// The oldest block that produced an entry a destination has not delivered
	owed, err := x.owedFrom(batch, current, oldest)
	if err != nil {
		return errors.UnknownError.WithFormat("find the undelivered entries: %w", err)
	}
	if current > maxSeedBlocks && owed < current-maxSeedBlocks {
		x.logger.Error("The seed would rebuild more blocks than its bound; entries older than the bound are not held and their destinations cannot be answered for them",
			"module", "synthetic", "owed-from", owed, "bound", maxSeedBlocks, "current", current)
		owed = current - maxSeedBlocks
	}

	// What the Directory has receipted of ours, newest first
	receipts, err := x.ownReceipts(batch, oldest, owed)
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
	if owed < from {
		from = owed
	}

	// A block a join carried this node past it did not execute: it produced
	// none of the block's synthetics and holds none of their messages, since
	// the join pulls <partition>/synthetic state-only. Such a block is not its
	// to rebuild, dispatch or serve (executor.md, "Sync" §6), and the store
	// says which blocks they are: a synthetic entry of the block with no
	// message behind it. That is the store's own fact, true after any number
	// of restarts, where a span remembered by the process that joined dies
	// with it (#4400). A block whose messages are there is rebuilt.
	//
	// Only that evidence skips a block, matched by identity: any other
	// absence in the rebuild -- a companion transaction, the root index
	// chain, a read past a windowed store's reach -- is a failure of a block
	// this node did execute, and fails the seed as loudly as before.
	var blocks []*synthcache.Block
	notExecuted := map[uint64]bool{}
	var skipped []uint64
	for b := from; b < current; b++ {
		blk, err := x.rebuildCacheBlock(batch, b)
		if ne, ok := err.(*notExecutedError); ok {
			notExecuted[b] = true
			skipped = append(skipped, b)
			x.logger.Info("Not rebuilding a block this node did not execute", "module", "synthetic", "block", b, "evidence", ne.Error())
			continue
		}
		if err != nil {
			return errors.UnknownError.WithFormat("rebuild cache for block %d: %w", b, err)
		}
		blocks = append(blocks, blk)
	}
	x.synthCache().Seed(blocks)
	if len(blocks) > 0 || len(skipped) > 0 {
		x.logger.Info("Seeded the synthetic cache from the chains", "module", "synthetic", "from", from, "owed-from", owed, "to", current-1, "blocks", len(blocks), "receipted", len(receipts), "skipped", len(skipped), "skipped-blocks", skipped)
	}
	err = x.seedProducedAnchors(batch)
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
		if r.block < from || notExecuted[r.block] {
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
// in-flight tail below the newest, or to oldest, and in either case down to
// floor, the oldest block holding an undelivered entry.
func (x *Executor) ownReceipts(batch *database.Batch, oldest, floor uint64) ([]ownReceipt, error) {
	c := batch.Account(x.Describe.AnchorPool()).MainChain()
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load anchor pool main chain head: %w", err)
	}
	own := x.Describe.PartitionUrl().URL
	var receipts []ownReceipt
	bound := min(oldest, floor)
	for i := head.Count - 1; i >= 0; i-- {
		entry, err := c.Entry(i)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			// The start of what the node holds: a join takes the pool's
			// recent window and backfills the rest (#4452). A receipt older
			// than the window is for a block before the join, which this
			// node did not execute and does not dispatch; a block it did
			// execute and finds no receipt for is sent again, and the
			// destination tosses what it has.
			x.logger.Info("The seed's walk of the anchor pool reached what the node does not hold", "module", "synthetic", "entry", i, "receipts", len(receipts))
			return receipts, nil
		default:
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
			bound = min(bound, floor)
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

// notExecutedError is the store's evidence that this node did not execute a
// block: a synthetic entry of it with no message behind it, or entries the
// index names that the store does not hold (#4400). It is its own type, and
// the seed matches it by type and only as rebuildCacheBlock returns it: a
// status code would match every NotFound in the rebuild, and would turn a
// block this node executed and cannot read into a silent skip.
type notExecutedError struct {
	dst      *url.URL
	evidence string
}

func (e *notExecutedError) Error() string {
	if e.dst == nil {
		return "this node did not execute the block: " + e.evidence
	}
	return fmt.Sprintf("this node did not execute the block: %s (stream to %v)", e.evidence, e.dst)
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
		case stderrors.Is(err, indexing.ErrNotHeld):
			// The index entries the block's lie among are not held: a join
			// took the chain's recent window and this block is below it.
			// A block this node executed wrote its index entry, and the
			// join holds the last entry before its match (#4452).
			return nil, &notExecutedError{dst, "the synthetic index chain entries around the block are not held"}
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
			if errors.Is(err, errors.NotFound) {
				// Held only from the block's own entry up: the block is the
				// oldest of a window a join took, and not this node's
				// (#4452).
				return nil, &notExecutedError{dst, fmt.Sprintf("synthetic index chain entry %d is not held", indexIndex-1)}
			}
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load synthetic index chain entry %d for %v: %w", indexIndex-1, dst, err)
			}
			from = int64(prev.Source) + 1
		}

		chain, err := c.Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic chain for %v: %w", dst, err)
		}

		// The entries and the messages behind them first, and the chain's
		// state before the block only after: they are the store's evidence
		// of whether this node executed the block, and the state is not. A
		// chain a join took as a head and an open mark set holds no mark
		// point below the set, so the state before a block in it is not
		// there to read -- and reading it first failed the whole seed on a
		// block the skip rule was written for (#4434, run 20260924T111811Z:
		// "mark point 1023 of …SyntheticChain.bvn2 is missing").
		hashes, err := chain.Entries(from, to+1)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			// The index names entries the store does not hold: the chain
			// came from a peer as a head and an open mark set.
			return nil, &notExecutedError{dst, fmt.Sprintf("synthetic chain entries %d..%d are not held", from, to)}
		default:
			return nil, errors.UnknownError.WithFormat("load synthetic chain entries %d..%d for %v: %w", from, to, dst, err)
		}
		seqs := make([]*messaging.SequencedMessage, len(hashes))
		for i, hash := range hashes {
			err := batch.Message2(hash).Main().GetAs(&seqs[i])
			switch {
			case err == nil:
			case errors.Is(err, errors.NotFound):
				// An entry with no message behind it: a node that executed
				// the block wrote both in the same batch.
				return nil, &notExecutedError{dst, fmt.Sprintf("synthetic entry %d (%x) has no message behind it", from+int64(i), hash[:4])}
			default:
				return nil, errors.UnknownError.WithFormat("load synthetic message: %w", err)
			}
		}

		before, err := chain.State(from - 1)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load synthetic chain state for %v before %d: %w", dst, from, err)
		}
		st := &synthcache.Stream{Destination: dst, ChainName: c.Name(), IndexIndex: indexIndex, RootPos: int64(indexEntry.Anchor), Segment: &merkle.Segment{First: from, Before: before, MarkMask: c.Inner().MarkMask()}}
		blk.Streams[synthcache.StreamKey(dst)] = st

		for i, hash := range hashes {
			seq := seqs[i]
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
	if stderrors.Is(err, indexing.ErrNotHeld) {
		// The root chain as a join took it, from its recent window: a block
		// this node executed wrote its root chain entries (#4452).
		return nil, &notExecutedError{evidence: fmt.Sprintf("the root index chain entries around block %d are not held", b)}
	}
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate block %d root index chain entry: %w", b, err)
	}
	root, err := ledger.RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load root chain: %w", err)
	}
	for _, st := range blk.Streams {
		st.RootReceipt, err = root.Receipt(st.RootPos, int64(rootEntry.Source))
		if errors.Is(err, errors.NotFound) {
			// The same: the root chain below its window is not held, and a
			// node that executed the block holds what it appended (#4452).
			return nil, &notExecutedError{st.Destination, fmt.Sprintf("the root chain entries %d..%d are not held", st.RootPos, rootEntry.Source)}
		}
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get root chain receipt from %d to %d: %w", st.RootPos, rootEntry.Source, err)
		}
	}
	return blk, nil
}

// seedProducedAnchors puts this partition's own anchors back in the cache at
// start (#4277): every anchor a destination has not delivered, from the
// lowest Delivered this node recorded for its destinations (SystemData
// AnchorDelivered; #4464). A produced anchor goes to every destination under
// one number — the Directory for a BVN, every partition for the Directory —
// so the lowest of them is what is still owed.
//
// The anchor sequence chain is the durable record of what this partition
// produced: entry i is the anchor with sequence number i+1. The cache that
// answers for them is memory, filled a block at a time as each anchor is
// produced, so without this a restart can serve nothing it produced before —
// and a destination that is behind on the stream asks, is refused, and stays
// behind, because there is no other place the anchor can come from.
func (x *Executor) seedProducedAnchors(batch *database.Batch) error {
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

	// The lowest any destination has delivered
	dests := []*url.URL{protocol.DnUrl()}
	if x.Describe.NetworkType == protocol.PartitionTypeDirectory {
		dests = nil
		for _, part := range x.globals().Active.Network.Partitions {
			dests = append(dests, protocol.PartitionUrl(part.ID))
		}
	}
	var delivered uint64
	for i, dst := range dests {
		n, err := batch.SystemData(x.Describe.PartitionId).AnchorDelivered(synthcache.StreamKey(dst)).Get()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.UnknownError.WithFormat("load %v's delivered anchor: %w", dst, err)
		}
		if i == 0 || n < delivered {
			delivered = n
		}
	}

	// Newest first, down to what every destination has delivered
	var anchors []synthcache.SeededAnchor
	for i := head.Count - 1; i >= 0 && uint64(i) >= delivered; i-- {
		if len(anchors) >= synthcache.MaxHeldAnchors {
			x.logger.Error("The seed reached the cache's anchor bound; older undelivered anchors are not held and their destinations cannot be answered for them",
				"module", "synthetic", "bound", synthcache.MaxHeldAnchors, "delivered", delivered, "oldest-held", uint64(i)+2)
			break
		}
		hash, err := chain.Entry(i)
		if errors.Is(err, errors.NotFound) {
			break // Below the window a join took (#4452): not held yet
		}
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
		anchors = append(anchors, synthcache.SeededAnchor{Number: uint64(i) + 1, Block: body.GetPartitionAnchor().MinorBlockIndex, Txn: txn.Transaction})
	}
	if len(anchors) == 0 {
		return nil
	}
	x.synthCache().SeedAnchors(anchors)
	x.logger.Info("Seeded produced anchors from the sequence chain", "module", "synthetic",
		"count", len(anchors), "from", anchors[len(anchors)-1].Number, "to", anchors[0].Number, "delivered", delivered)
	return nil
}

// maxSeedBlocks bounds how far back the seed rebuilds: the blocks it reads
// at start, and with them the time a start takes. Past it the seed logs an
// error, as the cache does at its memory bound.
const maxSeedBlocks = 100_000

// owedFrom is the oldest block that produced an entry a destination has not
// delivered: per stream, the block of the number after the destination's
// Delivered as this node recorded it, found on the stream's index chain.
// current when nothing is owed. A stream whose owed entries lie below the
// window a join took is owed from oldest: the node did not produce them.
func (x *Executor) owedFrom(batch *database.Batch, current, oldest uint64) (uint64, error) {
	owed := current
	record := batch.Account(x.Describe.Synthetic())
	sys := batch.SystemData(x.Describe.PartitionId)
	for _, part := range x.globals().Active.Network.Partitions {
		dst := protocol.PartitionUrl(part.ID)
		c := record.SyntheticChain(part.ID)
		head, err := c.Head().Get()
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load synthetic chain head for %v: %w", dst, err)
		}
		delivered, err := sys.SyntheticDelivered(synthcache.StreamKey(dst)).Get()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return 0, errors.UnknownError.WithFormat("load %v's delivered: %w", dst, err)
		}
		if int64(delivered) >= head.Count {
			continue
		}
		index, err := c.Index().Get()
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load synthetic index chain for %v: %w", dst, err)
		}
		if index.Height() == 0 {
			continue
		}
		// Entry i is number i+1; the block's index entry is the first whose
		// last position is at or past the entry's
		_, entry, err := indexing.SearchIndexChain(index, uint64(index.Height()-1), indexing.MatchAfter, indexing.SearchIndexChainBySource(delivered))
		switch {
		case err == nil:
			owed = min(owed, entry.BlockIndex)
		case stderrors.Is(err, indexing.ErrNotHeld):
			owed = min(owed, oldest)
		default:
			return 0, errors.UnknownError.WithFormat("locate synthetic %d for %v: %w", delivered+1, dst, err)
		}
	}
	return owed, nil
}

// seedDeep is seedCacheOnce over a read-only batch that reaches past a
// windowed store's window.
func (x *Executor) seedDeep(current uint64, isLeader bool) error {
	return x.deepView(func(batch *database.Batch) error {
		return x.seedCacheOnce(batch, current, isLeader)
	})
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

// ForgetSeed makes the next block to open seed the cache again, as the first
// block of a new process does. The daemon never calls it: a process start
// builds a new executor, whose latch is clear. The simulator has no process
// to start, and keeps one executor per node across a restart, so without it
// a restarted simulator node never ran the seed and a store the seed could
// not read passed every test (#4421).
func (x *Executor) ForgetSeed() {
	x.cacheSeedMu.Lock()
	defer x.cacheSeedMu.Unlock()
	x.cacheSeeded = false
}

// recordSettled writes, node-local, what the commit of block applies of the
// destinations' words (Cache.SettledBy): per destination, the number it has
// delivered of this partition's synthetic stream and of its anchor stream.
// Only ever forward: a word is the partition's, and a later block may apply
// an older one.
func (x *Executor) recordSettled(batch *database.Batch, block uint64) error {
	synthetic, anchors := x.synthCache().SettledBy(block)
	sys := batch.SystemData(x.Describe.PartitionId)
	put := func(v values.Value[uint64], through uint64) error {
		held, err := v.Get()
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
		default:
			return err
		}
		if through <= held {
			return nil
		}
		return v.Put(through)
	}
	for k, n := range synthetic {
		if err := put(sys.SyntheticDelivered(k), n); err != nil {
			return errors.UnknownError.WithFormat("record %s delivered %d: %w", k, n, err)
		}
	}
	for k, n := range anchors {
		if err := put(sys.AnchorDelivered(k), n); err != nil {
			return errors.UnknownError.WithFormat("record %s delivered anchor %d: %w", k, n, err)
		}
	}
	return nil
}
