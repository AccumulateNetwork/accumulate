// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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

func (x *ExecutorV1) LastBlock() (*execute.BlockParams, [32]byte, error) {
	batch := x.Database.Begin(false)
	defer batch.Discard()

	c, err := batch.Account(x.Describe.Ledger()).RootChain().Index().Get()
	if err != nil {
		return nil, [32]byte{}, errors.FatalError.WithFormat("load root index chain: %w", err)
	}
	if c.Height() == 0 {
		return nil, [32]byte{}, errors.NotFound
	}

	entry := new(protocol.IndexEntry)
	err = c.EntryAs(c.Height()-1, entry)
	if err != nil {
		return nil, [32]byte{}, errors.FatalError.WithFormat("load root index chain entry 0: %w", err)
	}

	b := new(BlockParams)
	b.Index = entry.BlockIndex
	b.Time = *entry.BlockTime

	h, err := batch.GetBptRootHash()
	return b, h, err
}

func (x *ExecutorV1) Init(validators []*ValidatorUpdate) (additional []*ValidatorUpdate, err error) {
	err = (*block.Executor)(x).Init(x.Database)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return (*block.Executor)(x).InitChainValidators(validators)
}

// Validate converts the message to a delivery and validates it. Validate
// returns an error if the message is not a [message.LegacyMessage].
func (x *ExecutorV1) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
	// The messages field does not exist in v1.0 so it must be ignored to
	// preserve the same behavior
	envelope.Messages = nil

	// Only use the shared batch when the check type is CheckTxType_New,
	//   we want to avoid changes to variables version increments to and stick and therefore be done multiple times
	var batch *database.Batch
	if recheck {
		batch = x.Database.Begin(false)
		defer batch.Discard()
	} else {
		// For cases where we haven't started/ended a block yet
		if x.CheckTxBatch == nil {
			x.CheckTxBatch = x.Database.Begin(false)
		}
		batch = x.CheckTxBatch
	}

	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	deliveries, err := chain.DeliveriesFromMessages(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	st := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		st[i] = new(protocol.TransactionStatus)

		if isHalted((*block.Executor)(x), delivery) {
			err = errors.NotAllowed.WithFormat("user messages are not being accepted: an upgrade is in progress")
		} else {
			st[i].Result, err = (*block.Executor)(x).ValidateEnvelope(batch, delivery)
		}

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
	err := x.EventBus.Publish(execute.WillBeginBlock{BlockParams: params})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	b := new(BlockV1)
	b.Executor = (*block.Executor)(x)
	b.Block = new(block.Block)
	b.Block.Batch = x.Database.Begin(true)
	b.Block.BlockMeta = params
	err = b.Executor.BeginBlock(b.Block)
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

func isHalted(x *block.Executor, delviery *chain.Delivery) bool {
	if !delviery.Transaction.Body.Type().IsUser() {
		return false
	}
	if !x.ActiveGlobals().ExecutorVersion.HaltV1() {
		return false
	}
	return delviery.Transaction.Body.Type() != protocol.TransactionTypeActivateProtocolVersion
}

// Process converts the message to a delivery and processes it. Process returns
// an error if the message is not a [message.LegacyMessage].
func (b *BlockV1) Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	// The messages field does not exist in v1.0 so it must be ignored to
	// preserve the same behavior
	envelope.Messages = nil

	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	deliveries, err := chain.DeliveriesFromMessages(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	st := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		if isHalted(b.Executor, delivery) {
			err = errors.NotAllowed.WithFormat("user messages are not being accepted: an upgrade is in progress")
		} else {
			st[i], err = b.Executor.ExecuteEnvelope(b.Block, delivery)
		}
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
	valUp, err := b.Executor.EndBlock(b.Block)
	return &BlockStateV1{*b, valUp}, err
}

// BlockStateV1 translates [execute.BlockState] calls for a v1 executor block.
type BlockStateV1 struct {
	BlockV1
	valUp []*execute.ValidatorUpdate
}

func (b *BlockStateV1) Params() execute.BlockParams { return b.Block.BlockMeta }

func (s *BlockStateV1) IsEmpty() bool {
	return s.Block.State.Empty()
}

func (s *BlockStateV1) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	return s.valUp, len(s.valUp) > 0
}

func (s *BlockStateV1) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return s.Block.State.MakeMajorBlock,
		s.Block.State.MakeMajorBlockTime,
		s.Block.State.MakeMajorBlock > 0
}

func (s *BlockStateV1) Hash() ([32]byte, error) {
	return s.Block.Batch.GetBptRootHash()
}

func (s *BlockStateV1) Commit() error {
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

	err = s.Block.Batch.Commit()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Start a new checkTx batch
	if s.Executor.CheckTxBatch != nil {
		s.Executor.CheckTxBatch.Discard()
	}
	s.Executor.CheckTxBatch = s.Executor.Database.Begin(false)
	return nil
}

func (s *BlockStateV1) Discard() {
	s.Block.Batch.Discard()
}

func (s *BlockStateV1) ChangeSet() record.Record {
	return s.Block.Batch
}
