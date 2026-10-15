// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"cmp"
	"slices"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (b *Block) shouldOpenMajorBlock() error {
	// Only the directory network can open a major block
	if b.Executor.Describe.NetworkType != protocol.PartitionTypeDirectory {
		return nil
	}

	var anchor *protocol.AnchorLedger
	err := b.Batch.Account(b.Executor.Describe.AnchorPool()).Main().GetAs(&anchor)
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}

	// The addition of a second here is kept for backwards compatibility
	blockTimeUTC := b.Time.Add(time.Second).UTC()
	nextBlockTime := b.Executor.globals().Active.MajorBlockSchedule().Next(anchor.MajorBlockTime)
	if blockTimeUTC.IsZero() || blockTimeUTC.Before(nextBlockTime) {
		return nil
	}

	b.State.MajorBlock = &MajorBlockState{
		Time:  blockTimeUTC,
		Index: anchor.MajorBlockIndex + 1,
	}
	return nil
}

func (b *Block) didOpenMajorBlock() (uint64, time.Time, bool) {
	if b.Executor.globals().Active.ExecutorVersion.V2VandenbergEnabled() &&
		b.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory {

		// On the DN, we opened a major block iff block.State.MajorBlock is set
		if b.State.MajorBlock == nil {
			return 0, time.Time{}, false
		}
		return b.State.MajorBlock.Index, b.State.MajorBlock.Time, true

	} else {
		// On a BVN, we opened a major block iff block.State.MakeMajorBlock is
		// set
		if b.State.MakeMajorBlock == 0 {
			return 0, time.Time{}, false
		}
		return b.State.MakeMajorBlock, b.State.MakeMajorBlockTime, true
	}
}

// recordMajorBlock updates major index chains.
func (x *Executor) recordMajorBlock(block *Block, rootIndexIndex uint64) error {
	// Is it time to record a major block?
	blockIndex, blockTime, ok := block.didOpenMajorBlock()
	if !ok {
		return nil
	}
	opened := []MajorBlockState{{Index: blockIndex, Time: blockTime}}

	// From v2-canaveral a BVN records every major block it learned of this
	// block, in order, and none at or below the last it recorded (executor
	// spec, "Major blocks on a BVN")
	account := block.Batch.Account(x.Describe.AnchorPool())
	if x.globals().Active.ExecutorVersion.V2CanaveralEnabled() &&
		x.Describe.NetworkType != protocol.PartitionTypeDirectory {
		last, err := lastMajorBlock(account.MajorBlockChain())
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		opened = majorBlocksAfter(last, block.State.MajorBlockNotices)
		if len(opened) == 0 {
			return nil
		}
	}

	// Load the chain
	mainChain, err := account.MainChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger main chain: %w", err)
	}

	for _, m := range opened {
		_, err = addIndexChainEntry(account.MajorBlockChain(), &protocol.IndexEntry{
			Source:         uint64(mainChain.Height() - 1),
			RootIndexIndex: rootIndexIndex,
			BlockIndex:     m.Index,
			BlockTime:      &m.Time,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("add anchor ledger index chain entry: %w", err)
		}
	}

	return nil
}

// lastMajorBlock is the index of the last major block the chain records, or
// zero when it records none.
func lastMajorBlock(c *database.Chain2) (uint64, error) {
	head, err := c.Head().Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load major block chain: %w", err)
	}
	if head.Count == 0 {
		return 0, nil
	}
	entry := new(protocol.IndexEntry)
	err = c.EntryAs(head.Count-1, entry)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load major block chain entry %d: %w", head.Count-1, err)
	}
	return entry.BlockIndex, nil
}

// majorBlocksAfter is the notices of major blocks after last, one per index,
// in increasing order.
func majorBlocksAfter(last uint64, notices []MajorBlockState) []MajorBlockState {
	var after []MajorBlockState
	for _, m := range notices {
		if m.Index > last {
			after = append(after, m)
		}
	}
	slices.SortStableFunc(after, func(a, b MajorBlockState) int { return cmp.Compare(a.Index, b.Index) })
	return slices.CompactFunc(after, func(a, b MajorBlockState) bool { return a.Index == b.Index })
}
