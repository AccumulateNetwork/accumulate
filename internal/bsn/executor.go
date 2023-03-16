// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var executors []func(ExecutorOptions) (messaging.MessageType, MessageExecutor)

type Executor struct {
	executors map[messaging.MessageType]MessageExecutor
	logger    logging.OptionalLogger
	store     storage.Beginner
}

type ExecutorOptions struct {
	Logger log.Logger
	Store  storage.Beginner
}

func NewExecutor(opts ExecutorOptions) (*Executor, error) {
	x := new(Executor)
	x.logger.Set(opts.Logger, "module", "executor")
	x.store = opts.Store
	x.executors = newExecutorMap(opts, executors)
	return x, nil
}

func (*Executor) EnableTimers()                                                  {}
func (*Executor) StoreBlockTimers(ds *logging.DataSet)                           {}
func (*Executor) LoadStateRoot(*database.Batch) ([]byte, error)                  { return nil, nil }
func (*Executor) RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error { return nil }
func (*Executor) InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error) {
	return nil, nil
}

func (*Executor) Validate(*database.Batch, []messaging.Message) ([]*protocol.TransactionStatus, error) {
	return nil, nil
}
