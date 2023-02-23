// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// Block implements [execute.Block].
type Block struct {
	execute.BlockParams
	State    BlockState
	Batch    *database.Batch
	Executor *Executor
}

func (b *Block) Params() execute.BlockParams { return b.BlockParams }

// Close ends the block and returns the block state.
func (b *Block) Close() (execute.BlockState, error) {
	err := b.Executor.EndBlock(b)
	return (*closedBlock)(b), err
}

// closedBlock implements [execute.BlockState].
type closedBlock Block

func (b *closedBlock) Params() execute.BlockParams { return b.BlockParams }

func (s *closedBlock) IsEmpty() bool {
	return s.State.Empty()
}

func (s *closedBlock) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.State.MakeMajorBlock,
		s.State.MakeMajorBlockTime,
		s.State.MakeMajorBlock > 0
}

func (s *closedBlock) Commit() error {
	return s.Batch.Commit()
}

func (s *closedBlock) Discard() {
	s.Batch.Discard()
}