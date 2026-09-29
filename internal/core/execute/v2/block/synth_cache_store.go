// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	stderrors "errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The executor is the cache's store: what the cache no longer holds, a
// destination that still lacks it is answered from the partition's own
// chains (healing spec, "The cache"; #4464). A whole-network restart seeds
// each cache only back to the Directory's newest receipt, and a stream held
// longer than the horizon is trimmed while it is held; in both, the entries
// the destination is missing were produced, are on the chains, and no peer
// remembers them.
var _ synthcache.Store = (*Executor)(nil)

// SyntheticBlock rebuilds the block that produced synthetic number of the
// stream to dst, as the seed does (rebuildCacheBlock), dispatched under the
// Directory anchor that receipted it.
func (x *Executor) SyntheticBlock(dst *url.URL, number uint64) (*synthcache.Block, error) {
	id, ok := protocol.ParsePartitionUrl(dst)
	if !ok {
		return nil, errors.BadRequest.WithFormat("%v is not a partition", dst)
	}
	var blk *synthcache.Block
	err := x.deepView(func(batch *database.Batch) error {
		c := batch.Account(x.Describe.Synthetic()).SyntheticChain(id)
		head, err := c.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic chain head for %v: %w", dst, err)
		}
		// Entry i of the chain is number i+1
		if number == 0 || int64(number) > head.Count {
			return errors.NotFound.WithFormat("synthetic %d for %v is not produced", number, dst)
		}
		index, err := c.Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic index chain for %v: %w", dst, err)
		}
		if index.Height() == 0 {
			return errors.NotFound.WithFormat("synthetic %d for %v is not indexed", number, dst)
		}
		// The block's index entry is the first whose last position is at or
		// past the entry's
		_, entry, err := indexing.SearchIndexChain(index, uint64(index.Height()-1), indexing.MatchAfter, indexing.SearchIndexChainBySource(number-1))
		switch {
		case err == nil:
		case stderrors.Is(err, indexing.ErrNotHeld):
			return errors.NotFound.WithFormat("the synthetic index chain entries for %v around %d are not held", dst, number)
		default:
			return errors.UnknownError.WithFormat("locate synthetic %d for %v: %w", number, dst, err)
		}

		b, err := x.rebuildCacheBlock(batch, entry.BlockIndex)
		if _, ok := err.(*notExecutedError); ok {
			return synthcache.ErrNotExecuted
		}
		if err != nil {
			return errors.UnknownError.WithFormat("rebuild block %d: %w", entry.BlockIndex, err)
		}
		anchorBlock, receipt, err := x.dispatchedUnder(batch, entry.BlockIndex)
		if err != nil {
			return err
		}
		b.Dispatched, b.AnchorBlock, b.DirectoryReceipt = true, anchorBlock, receipt
		blk = b
		return nil
	})
	return blk, err
}

// dispatchedUnder finds the Directory anchor this partition executed that
// receipted own block b: the anchor the block's synthetics were dispatched
// under, found in the anchor pool as the seed finds it (ownReceipts). The
// Directory receipts own blocks in order, so the anchor is found by bisecting
// the pool on the newest own block each Directory anchor receipts.
func (x *Executor) dispatchedUnder(batch *database.Batch, b uint64) (uint64, *protocol.PartitionAnchorReceipt, error) {
	c := batch.Account(x.Describe.AnchorPool()).MainChain()
	head, err := c.Head().Get()
	if err != nil {
		return 0, nil, errors.UnknownError.WithFormat("load anchor pool main chain head: %w", err)
	}

	// What entry i receipts of ours; nil for any other entry, and for one
	// below the window a join took (#4452), which is older than any answer
	receiptsAt := func(i int64) ([]ownReceipt, bool, error) {
		entry, err := c.Entry(i)
		if errors.Is(err, errors.NotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, errors.UnknownError.WithFormat("load anchor pool main chain entry %d: %w", i, err)
		}
		var msg *messaging.TransactionMessage
		err = batch.Message2(entry).Main().GetAs(&msg)
		if err != nil {
			return nil, false, errors.UnknownError.WithFormat("load anchor pool main chain entry %d: %w", i, err)
		}
		body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			return nil, true, nil
		}
		return x.ownReceiptsOf(body), true, nil
	}
	newest := func(rs []ownReceipt) uint64 {
		var n uint64
		for _, r := range rs {
			if r.block > n {
				n = r.block
			}
		}
		return n
	}

	// The first entry receipting a block at or past b
	var found []ownReceipt
	lo, hi := int64(0), head.Count
	for lo < hi {
		mid := lo + (hi-lo)/2
		j := mid
		var rs []ownReceipt
		for ; j < hi; j++ {
			var held bool
			rs, held, err = receiptsAt(j)
			if err != nil {
				return 0, nil, err
			}
			if !held {
				break
			}
			if len(rs) > 0 {
				break
			}
		}
		switch {
		case j < hi && len(rs) == 0:
			lo = j + 1 // not held: below the window
		case j == hi:
			hi = mid // nothing receipted in [mid, hi)
		case newest(rs) < b:
			lo = j + 1
		default:
			found, hi = rs, mid
		}
	}
	if found == nil {
		return 0, nil, errors.NotReady.WithFormat("the directory has not receipted block %d yet", b)
	}
	for _, r := range found {
		if r.block == b {
			return r.anchorBlock, r.receipt, nil
		}
	}
	return 0, nil, errors.NotFound.WithFormat("no Directory anchor held here receipts block %d", b)
}

// ProducedAnchor reads produced anchor number from the anchor sequence chain,
// as seedProducedAnchors does.
func (x *Executor) ProducedAnchor(number uint64) (*protocol.Transaction, uint64, error) {
	var txn *protocol.Transaction
	var block uint64
	err := x.deepView(func(batch *database.Batch) error {
		record := batch.Account(x.Describe.AnchorPool()).AnchorSequenceChain()
		head, err := record.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor sequence chain head: %w", err)
		}
		if number == 0 || int64(number) > head.Count {
			return errors.NotFound.WithFormat("anchor %d is not produced", number)
		}
		chain, err := record.Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor sequence chain: %w", err)
		}
		hash, err := chain.Entry(int64(number) - 1)
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor sequence chain entry %d: %w", number-1, err)
		}
		var msg *messaging.TransactionMessage
		err = batch.Message2(hash).Main().GetAs(&msg)
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor %d: %w", number, err)
		}
		body, ok := msg.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			return errors.InternalError.WithFormat("anchor %d is a %v", number, msg.Transaction.Body.Type())
		}
		txn, block = msg.Transaction, body.GetPartitionAnchor().MinorBlockIndex
		return nil
	})
	return txn, block, err
}
