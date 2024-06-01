// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"time"

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
	nextBlockTime := b.Executor.globals.Active.MajorBlockSchedule().Next(anchor.MajorBlockTime)
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
	if b.Executor.globals.Active.ExecutorVersion.V2VandenbergEnabled() &&
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

	// Load the chain
	account := block.Batch.Account(x.Describe.AnchorPool())
	mainChain, err := account.MainChain().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger main chain: %w", err)
	}

	_, err = addIndexChainEntry(account.MajorBlockChain(), &protocol.IndexEntry{
		Source:         uint64(mainChain.Height() - 1),
		RootIndexIndex: rootIndexIndex,
		BlockIndex:     blockIndex,
		BlockTime:      &blockTime,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("add anchor ledger index chain entry: %w", err)
	}

	return nil
}
