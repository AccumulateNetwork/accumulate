// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ExecutorV1 block.Executor

func (x *ExecutorV1) SetBackgroundTaskManager(f func(func())) {
	x.BackgroundTaskLauncher = f
}

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

func (x *ExecutorV1) Validate(batch *database.Batch, message messaging.Message) (*protocol.TransactionStatus, error) {
	legacy, ok := message.(*messaging.LegacyMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("unsupported message type: expected %v, got %v", messaging.MessageTypeLegacy, message.Type())
	}

	status := new(protocol.TransactionStatus)
	var err error
	status.Result, err = (*block.Executor)(x).ValidateEnvelope(batch, chain.DeliveryFromMessage(legacy))

	// Wait until after ValidateEnvelope, because the transaction may get
	// loaded by LoadTransaction
	status.TxID = legacy.Transaction.ID()
	return status, err
}

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

type BlockV1 struct {
	Block    *block.Block
	Executor *block.Executor
}

func (b *BlockV1) Params() execute.BlockParams { return b.Block.BlockMeta }

func (b *BlockV1) Process(message messaging.Message) (*protocol.TransactionStatus, error) {
	legacy, ok := message.(*messaging.LegacyMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("unsupported message type: expected %v, got %v", messaging.MessageTypeLegacy, message.Type())
	}

	status, err := b.Executor.ExecuteEnvelope(b.Block, chain.DeliveryFromMessage(legacy))
	if status == nil {
		status = new(protocol.TransactionStatus)
	}

	// Wait until after ValidateEnvelope, because the transaction may get
	// loaded by LoadTransaction
	status.TxID = legacy.Transaction.ID()
	return status, err
}

func (b *BlockV1) Close() (execute.BlockState, error) {
	err := b.Executor.EndBlock(b.Block)
	return (*BlockStateV1)(b.Block), err
}

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
