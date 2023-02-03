// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sync"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	v1 "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Alias these types to minimize imports

type Executor = execute.Executor
type Dispatcher = execute.Dispatcher
type Block = execute.Block
type BlockParams = execute.BlockParams
type BlockState = execute.BlockState
type Options = execute.Options

// NewExecutor creates a new executor.
func NewExecutor(opts Options) (Executor, error) {
	// Get the current version
	part := config.NetworkUrl{URL: protocol.PartitionUrl(opts.Describe.PartitionId)}
	var ledger *protocol.SystemLedger
	err := opts.Database.View(func(batch *database.Batch) error {
		return batch.Account(part.Ledger()).Main().GetAs(&ledger)
	})
	switch {
	case err == nil, errors.Is(err, errors.NotFound):
		// Ok
	default:
		return nil, errors.UnknownError.WithFormat("load ledger: %w", err)
	}

	// If the version is V2, create a V2 executor
	if ledger != nil && ledger.ExecutorVersion.V2() {
		exec, err := v2.NewNodeExecutor(opts)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("create v2 executor: %w", err)
		}
		return (*ExecutorV2)(exec), nil
	}

	exec, err := v1.NewNodeExecutor(opts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create v1 executor: %w", err)
	}

	x := new(Multi)
	x.mu = new(sync.RWMutex)
	x.opts = opts
	x.active = (*ExecutorV1)(exec)

	// Must be synchronous to avoid race conditions
	events.SubscribeSync(opts.EventBus, x.willChangeGlobals)

	return x, nil
}

type Multi struct {
	mu     *sync.RWMutex
	opts   Options
	active Executor
}

func (m *Multi) willChangeGlobals(e events.WillChangeGlobals) error {
	if !e.New.ExecutorVersion.V2() {
		return nil
	}

	// TODO Can we move this call into [NewExecutor] to reduce the possibility
	// of running into an error here?
	exec, err := v2.NewNodeExecutor(m.opts)
	if err != nil {
		return errors.UnknownError.WithFormat("create v2 executor: %w", err)
	}

	m.mu.Lock()
	m.active = (*ExecutorV2)(exec)
	m.mu.Unlock()
	return nil
}

func (m *Multi) EnableTimers() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.active.EnableTimers()
}

func (m *Multi) StoreBlockTimers(ds *logging.DataSet) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.active.StoreBlockTimers(ds)
}

func (m *Multi) LoadStateRoot(batch *database.Batch) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active.LoadStateRoot(batch)
}

func (m *Multi) RestoreSnapshot(db database.Beginner, snapshot ioutil2.SectionReader) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active.RestoreSnapshot(db, snapshot)
}

func (m *Multi) InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active.InitChainValidators(initVal)
}

func (m *Multi) Validate(batch *database.Batch, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active.Validate(batch, messages)
}

func (m *Multi) Begin(params BlockParams) (Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active.Begin(params)
}
