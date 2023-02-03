// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ExecutorV2 implements [execute.Executor] calls for a v1 executor.
type ExecutorV2 Executor

func (x *ExecutorV2) EnableTimers() {
	(*Executor)(x).EnableTimers()
}

func (x *ExecutorV2) StoreBlockTimers(ds *logging.DataSet) {
	(*Executor)(x).StoreBlockTimers(ds)
}

func (x *ExecutorV2) LoadStateRoot(batch *database.Batch) ([]byte, error) {
	return (*Executor)(x).LoadStateRoot(batch)
}

func (x *ExecutorV2) RestoreSnapshot(batch database.Beginner, snapshot ioutil2.SectionReader) error {
	return (*Executor)(x).RestoreSnapshot(batch, snapshot)
}

func (x *ExecutorV2) InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error) {
	return (*Executor)(x).InitChainValidators(initVal)
}

// Validate converts the message to a delivery and validates it. Validate
// returns an error if the message is not a [message.LegacyMessage].
func (x *ExecutorV2) Validate(batch *database.Batch, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	deliveries, err := chain.DeliveriesFromMessages(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	st := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		st[i] = new(protocol.TransactionStatus)
		st[i].Result, err = (*Executor)(x).ValidateEnvelope(batch, delivery)

		if err != nil {
			st[i].Set(err)
		}

		// Wait until after ValidateEnvelope, because the transaction may get
		// loaded by LoadTransaction
		st[i].TxID = delivery.Transaction.ID()
	}

	return st, nil
}

// Begin constructs a [BlockV2] and calls [Executor.BeginBlock].
func (x *ExecutorV2) Begin(params execute.BlockParams) (execute.Block, error) {
	b := new(BlockV2)
	b.Executor = (*Executor)(x)
	b.Block = new(Block)
	b.Block.Batch = x.Database.Begin(true)
	b.Block.BlockMeta = params
	err := b.Executor.BeginBlock(b.Block)
	if err != nil {
		b.Block.Batch.Discard()
	}
	return b, err
}

// BlockV2 translates [execute.Block] calls for a v1 executor/
type BlockV2 struct {
	Block    *Block
	Executor *Executor
}

func (b *BlockV2) Params() execute.BlockParams { return b.Block.BlockMeta }

// Close ends the block and returns the block state.
func (b *BlockV2) Close() (execute.BlockState, error) {
	err := b.Executor.EndBlock(b.Block)
	return (*BlockStateV2)(b.Block), err
}

// BlockStateV2 translates [execute.BlockState] calls for a v1 executor
type BlockStateV2 Block

func (b *BlockStateV2) Params() execute.BlockParams { return b.BlockMeta }

func (s *BlockStateV2) IsEmpty() bool {
	return s.State.Empty()
}

func (s *BlockStateV2) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.State.MakeMajorBlock,
		s.State.MakeMajorBlockTime,
		s.State.MakeMajorBlock > 0
}

func (s *BlockStateV2) Commit() error {
	return s.Batch.Commit()
}

func (s *BlockStateV2) Discard() {
	s.Batch.Discard()
}
