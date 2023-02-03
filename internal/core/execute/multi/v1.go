// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ExecutorV1 implements [execute.Executor] calls for a v1 executor.
type ExecutorV1 block.Executor

func (x *ExecutorV1) EnableTimers() {
	(*block.Executor)(x).EnableTimers()
}

func (x *ExecutorV1) StoreBlockTimers(ds *logging.DataSet) {
	(*block.Executor)(x).StoreBlockTimers(ds)
}

func (x *ExecutorV1) LoadStateRoot(batch *database.Batch) ([]byte, error) {
	return (*block.Executor)(x).LoadStateRoot(batch)
}

func (x *ExecutorV1) RestoreSnapshot(batch database.Beginner, snapshot ioutil2.SectionReader) error {
	return (*block.Executor)(x).RestoreSnapshot(batch, snapshot)
}

func (x *ExecutorV1) InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error) {
	return (*block.Executor)(x).InitChainValidators(initVal)
}

// Validate converts the message to a delivery and validates it. Validate
// returns an error if the message is not a [message.LegacyMessage].
func (x *ExecutorV1) Validate(batch *database.Batch, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	deliveries, err := chain.DeliveriesFromMessages(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	st := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		st[i] = new(protocol.TransactionStatus)
		st[i].Result, err = (*block.Executor)(x).ValidateEnvelope(batch, delivery)

		if err != nil {
			st[i].Set(err)
		}

		// Wait until after ValidateEnvelope, because the transaction may get
		// loaded by LoadTransaction
		st[i].TxID = delivery.Transaction.ID()
	}

	return st, nil
}

// Begin constructs a [BlockV1] and calls [block.Executor.BeginBlock].
func (x *ExecutorV1) Begin(params execute.BlockParams) (execute.Block, error) {
	b := new(BlockV1)
	b.Executor = (*block.Executor)(x)
	b.Block = new(block.Block)
	b.Block.Batch = x.Database.Begin(true)
	b.Block.BlockMeta = params
	err := b.Executor.BeginBlock(b.Block)
	if err != nil {
		b.Block.Batch.Discard()
	}
	return b, err
}

// BlockV1 translates [execute.Block] calls for a v1 executor/block.
type BlockV1 struct {
	Block    *block.Block
	Executor *block.Executor
}

func (b *BlockV1) Params() execute.BlockParams { return b.Block.BlockMeta }

// Process converts the message to a delivery and processes it. Process returns
// an error if the message is not a [message.LegacyMessage].
func (b *BlockV1) Process(messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	deliveries, err := chain.DeliveriesFromMessages(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	st := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		st[i], err = b.Executor.ExecuteEnvelope(b.Block, delivery)
		if st[i] == nil {
			st[i] = new(protocol.TransactionStatus)
		}

		if err != nil {
			st[i].Set(err)
		}

		// Wait until after ExecuteEnvelope, because the transaction may get
		// loaded by LoadTransaction
		st[i].TxID = delivery.Transaction.ID()
	}

	return st, nil
}

// Close ends the block and returns the block state.
func (b *BlockV1) Close() (execute.BlockState, error) {
	err := b.Executor.EndBlock(b.Block)
	return (*BlockStateV1)(b.Block), err
}

// BlockStateV1 translates [execute.BlockState] calls for a v1 executor block.
type BlockStateV1 block.Block

func (b *BlockStateV1) Params() execute.BlockParams { return b.BlockMeta }

func (s *BlockStateV1) IsEmpty() bool {
	return s.State.Empty()
}

func (s *BlockStateV1) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.State.MakeMajorBlock,
		s.State.MakeMajorBlockTime,
		s.State.MakeMajorBlock > 0
}

func (s *BlockStateV1) Commit() error {
	return s.Batch.Commit()
}

func (s *BlockStateV1) Discard() {
	s.Batch.Discard()
}
