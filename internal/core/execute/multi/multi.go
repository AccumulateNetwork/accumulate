// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sync/atomic"

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
		exec, err := v2.NewExecutor(opts)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("create v2 executor: %w", err)
		}
		return (*v2.ExecutorV2)(exec), nil
	}

	exec, err := v1.NewNodeExecutor(opts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create v1 executor: %w", err)
	}

	x := new(Multi)
	x.opts = opts
	x.setActive((*ExecutorV1)(exec))

	// Must be synchronous to avoid races
	events.SubscribeSync(opts.EventBus, x.willChangeGlobals)

	return x, nil
}

type Multi struct {
	opts                Options
	version, newVersion protocol.ExecutorVersion // Can be non-atomic because they are read and written synchronously within a block
	active              atomic.Pointer[Executor] // Use an atomic pointer to avoid races
}

func (m *Multi) setActive(exec Executor) {
	m.active.Store(&exec)
}

func (m *Multi) willChangeGlobals(e events.WillChangeGlobals) error {
	if !e.New.ExecutorVersion.V2() {
		return nil
	}

	// No need to update
	if m.version.V2() {
		return nil
	}

	m.newVersion = e.New.ExecutorVersion
	return nil
}

// updateActive updates the active executor version if the previous block
// changed it. This is only exposed for the simulator
func (m *Multi) updateActive() error {
	if m.version >= m.newVersion {
		return nil
	}
	m.version = m.newVersion

	// TODO Can we move this call into [NewExecutor] to reduce the possibility
	// of running into an error here?
	exec, err := v2.NewExecutor(m.opts)
	if err != nil {
		return errors.UnknownError.WithFormat("create v2 executor: %w", err)
	}

	m.setActive((*v2.ExecutorV2)(exec))
	return nil
}

func (m *Multi) EnableTimers() {
	(*m.active.Load()).EnableTimers()
}

func (m *Multi) StoreBlockTimers(ds *logging.DataSet) {
	(*m.active.Load()).StoreBlockTimers(ds)
}

func (m *Multi) LoadStateRoot(batch *database.Batch) ([]byte, error) {
	return (*m.active.Load()).LoadStateRoot(batch)
}

func (m *Multi) RestoreSnapshot(db database.Beginner, snapshot ioutil2.SectionReader) error {
	err := (*m.active.Load()).RestoreSnapshot(db, snapshot)
	if err != nil || m.version >= m.newVersion {
		return err
	}

	if db != m.opts.Database {
		// If db is not the same one used for initializing the executor,
		// NewExecutor will not see changes from RestoreSnapshot. TODO We
		// probably should just remove the db parameter from this function.
		return errors.Conflict.With("cannot activate the new executor version")
	}

	// Change the active executor implementation
	err = m.updateActive()
	return errors.UnknownError.Wrap(err)
}

func (m *Multi) InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error) {
	return (*m.active.Load()).InitChainValidators(initVal)
}

func (m *Multi) Validate(batch *database.Batch, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	return (*m.active.Load()).Validate(batch, messages)
}

func (m *Multi) Begin(params BlockParams) (Block, error) {
	b, err := (*m.active.Load()).Begin(params)
	if err != nil {
		return nil, err
	}
	return &multiBlock{m, b}, nil
}

type multiBlock struct {
	multi *Multi
	Block
}

func (b *multiBlock) Close() (BlockState, error) {
	s, err := b.Block.Close()
	if err != nil {
		return s, err
	}

	// Change the active executor implementation at the end of the block
	err = b.multi.updateActive()
	return s, errors.UnknownError.Wrap(err)
}
