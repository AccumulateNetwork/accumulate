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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Block implements [execute.Block].
type Block struct {
	execute.BlockParams
	State    BlockState
	Batch    *database.Batch
	Executor *Executor
}

func (b *Block) Params() execute.BlockParams { return b.BlockParams }

// closedBlock implements [execute.BlockState].
type closedBlock struct {
	Block
	valUp []*execute.ValidatorUpdate
}

func (b *closedBlock) Params() execute.BlockParams { return b.BlockParams }
func (s *closedBlock) ChangeSet() record.Record    { return s.Batch }
func (s *closedBlock) IsEmpty() bool               { return s.State.Empty() }

func (s *closedBlock) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.State.MakeMajorBlock,
		s.State.MakeMajorBlockTime,
		s.State.MakeMajorBlock > 0
}

func (s *closedBlock) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	return s.valUp, len(s.valUp) > 0
}

func (s *closedBlock) Hash() ([32]byte, error) {
	return s.Batch.GetBptRootHash()
}

func (s *closedBlock) Commit() error {
	if s.IsEmpty() {
		s.Discard()
		return nil
	}

	err := s.Executor.EventBus.Publish(execute.WillCommitBlock{
		Block: s,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return s.Batch.Commit()
}

func (s *closedBlock) Discard() {
	s.Batch.Discard()
}
