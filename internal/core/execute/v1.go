// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ExecutorV1 block.Executor

func (x *ExecutorV1) SetBackgroundTaskManager(f func(func())) {
	x.Background = f
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

func (x *ExecutorV1) InitChainValidators(initVal []abci.ValidatorUpdate) (additional [][]byte, err error) {
	return (*block.Executor)(x).InitChainValidators(initVal)
}

func (x *ExecutorV1) ValidateEnvelope(batch *database.Batch, message messaging.Message) (*protocol.TransactionStatus, error) {
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

func (x *ExecutorV1) BeginBlock(b *block.Block) error {
	return (*block.Executor)(x).BeginBlock(b)
}

func (x *ExecutorV1) ExecuteEnvelope(b *block.Block, message messaging.Message) (*protocol.TransactionStatus, error) {
	legacy, ok := message.(*messaging.LegacyMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("unsupported message type: expected %v, got %v", messaging.MessageTypeLegacy, message.Type())
	}

	status, err := (*block.Executor)(x).ExecuteEnvelope(b, chain.DeliveryFromMessage(legacy))
	if status == nil {
		status = new(protocol.TransactionStatus)
	}

	// Wait until after ValidateEnvelope, because the transaction may get
	// loaded by LoadTransaction
	status.TxID = legacy.Transaction.ID()
	return status, err
}

func (x *ExecutorV1) EndBlock(b *block.Block) error {
	return (*block.Executor)(x).EndBlock(b)
}
