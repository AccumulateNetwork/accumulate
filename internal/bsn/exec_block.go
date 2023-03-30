// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"time"

	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type blockStats struct {
	Processed int
}

func (s *blockStats) IsEmpty() bool {
	return s.Processed == 0
}

type Block struct {
	executor *Executor
	params   *execute.BlockParams
	batch    *ChangeSet
	stats    blockStats
}

func (x *Executor) Begin(params execute.BlockParams) (execute.Block, error) {
	b := new(Block)
	b.executor = x
	b.params = &params
	b.batch = NewChangeSet(x.store, x.logger)
	return b, nil
}

func (b *Block) Params() execute.BlockParams {
	if b.params == nil {
		return execute.BlockParams{}
	}
	return *b.params
}

func (b *Block) Close() (execute.BlockState, error) {
	if b.params == nil {
		return nil, errors.NotAllowed.With("not a block")
	}

	err := b.batch.LastBlock().Put(&LastBlock{
		Index: b.params.Index,
		Time:  b.params.Time,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store last block info: %w", err)
	}

	s := new(BlockState)
	s.params = *b.params
	s.batch = b.batch
	s.stats = b.stats
	return s, nil
}

type BlockState struct {
	params execute.BlockParams
	batch  *ChangeSet
	stats  blockStats
}

func (b *BlockState) Params() execute.BlockParams       { return b.params }
func (b *BlockState) IsEmpty() bool                     { return b.stats.IsEmpty() }
func (b *BlockState) Discard()                          { b.batch.Discard() }
func (b *BlockState) Hash() []byte                      { return nil }
func (b *BlockState) WalkChanges(record.WalkFunc) error { return nil }

func (b *BlockState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return 0, time.Time{}, false
}

func (b *BlockState) Commit() error {
	if b.IsEmpty() {
		b.Discard()
		return nil
	}
	return b.batch.Commit()
}
