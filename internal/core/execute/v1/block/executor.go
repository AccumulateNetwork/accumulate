// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/ed25519"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Executor struct {
	ExecutorOptions
	BlockTimers TimerSet

	globals        *Globals
	executors      map[protocol.TransactionType]chain.TransactionExecutor
	logger         logging.OptionalLogger
	db             database.Beginner
	isValidator    bool
	isGenesis      bool
	mainDispatcher Dispatcher
}

type ExecutorOptions = execute.Options
type Dispatcher = execute.Dispatcher

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions) (*Executor, error) {
	executors := []chain.TransactionExecutor{
		// User transactions
		chain.AddCredits{},
		chain.BurnTokens{},
		chain.CreateDataAccount{},
		chain.CreateIdentity{},
		chain.CreateKeyBook{},
		chain.CreateKeyPage{},
		chain.CreateLiteTokenAccount{},
		chain.CreateToken{},
		chain.CreateTokenAccount{},
		chain.IssueTokens{},
		chain.LockAccount{},
		chain.SendTokens{},
		chain.UpdateAccountAuth{},
		chain.UpdateKey{},
		chain.UpdateKeyPage{},
		chain.WriteData{},
		chain.WriteDataTo{},

		// Synthetic
		chain.SyntheticBurnTokens{},
		chain.SyntheticCreateIdentity{},
		chain.SyntheticDepositCredits{},
		chain.SyntheticDepositTokens{},
		chain.SyntheticWriteData{},

		// Forwarding
		chain.SyntheticForwardTransaction{},

		// Operator transactions
		chain.ActivateProtocolVersion{},
	}

	switch opts.Describe.NetworkType {
	case config.Directory:
		executors = append(executors,
			chain.PartitionAnchor{},
			chain.DirectoryAnchor{},
		)

	case config.BlockValidator:
		executors = append(executors,
			chain.DirectoryAnchor{},
		)

	default:
		return nil, errors.InternalError.WithFormat("invalid partition type %v", opts.Describe.NetworkType)
	}

	// This is a no-op in dev
	executors = addTestnetExecutors(executors)

	return newExecutor(opts, false, executors...)
}

// NewGenesisExecutor creates a transaction executor that can be used to set up
// the genesis state.
func NewGenesisExecutor(db *database.Database, logger log.Logger, network *config.Describe, globals *core.GlobalValues, router routing.Router) (*Executor, error) {
	exec, err := newExecutor(
		ExecutorOptions{
			Database: db,
			Describe: *network,
			Logger:   logger,
			Router:   router,
			EventBus: events.NewBus(logger),
		},
		true,
		chain.SystemWriteData{},
	)
	if err != nil {
		return nil, err
	}
	exec.globals = new(Globals)
	exec.globals.Pending = *globals
	exec.globals.Active = *globals
	return exec, nil
}

func newExecutor(opts ExecutorOptions, isGenesis bool, executors ...chain.TransactionExecutor) (*Executor, error) {
	if opts.BackgroundTaskLauncher == nil {
		opts.BackgroundTaskLauncher = func(f func()) { go f() }
	}
	if isGenesis {
		opts.NewDispatcher = func() Dispatcher { return nullDispatcher{} }
	}

	m := new(Executor)
	m.ExecutorOptions = opts
	m.executors = map[protocol.TransactionType]chain.TransactionExecutor{}
	m.db = opts.Database
	m.mainDispatcher = opts.NewDispatcher()
	m.isGenesis = isGenesis

	if opts.Logger != nil {
		m.logger.L = opts.Logger.With("module", "executor")
	}

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(errors.InternalError.WithFormat("duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x
	}

	batch := opts.Database.Begin(false)
	defer batch.Discard()

	// Listen to our own event (DRY)
	events.SubscribeSync(m.EventBus, func(e events.WillChangeGlobals) error {
		_, v, _ := e.New.Network.ValidatorByKey(m.Key[32:])
		m.isValidator = v.IsActiveOn(m.Describe.PartitionId)
		return nil
	})

	// Load globals if the database has been initialized
	var ledger *protocol.SystemLedger
	err := batch.Account(m.Describe.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		// Database has been initialized
		// m.logger.Debug("Loaded", "height", ledger.Index, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))

		// Load globals
		err = m.loadGlobals(opts.Database.View)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

	case errors.Is(err, storage.ErrNotFound):
		// Database is uninitialized
		// m.logger.Debug("Loaded", "height", 0, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))

	default:
		return nil, errors.UnknownError.WithFormat("load ledger: %w", err)
	}

	return m, nil
}

func (m *Executor) EnableTimers() {
	m.BlockTimers.Initialize(&m.executors)
}

func (m *Executor) StoreBlockTimers(ds *logging.DataSet) {
	m.BlockTimers.Store(ds)
}

func (m *Executor) ActiveGlobals_TESTONLY() *core.GlobalValues {
	return &m.globals.Active
}

func (x *Executor) SetExecutor_TESTONLY(y chain.TransactionExecutor) {
	x.executors[y.Type()] = y
}

func (m *Executor) Genesis(block *Block, exec chain.TransactionExecutor) error {
	var err error

	if !m.isGenesis {
		panic("Cannot call Genesis on a node txn executor")
	}
	m.executors[protocol.TransactionTypeSystemGenesis] = exec

	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AcmeUrl()
	txn.Body = new(protocol.SystemGenesis)
	delivery := new(chain.Delivery)
	delivery.Transaction = txn

	st := chain.NewStateManager(&m.Describe, nil, block.Batch.Begin(true), nil, txn, m.logger.With("operation", "Genesis"))
	defer st.Discard()

	err = block.Batch.Transaction(txn.GetHash()).PutStatus(&protocol.TransactionStatus{
		Initiator: txn.Header.Principal,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	status, err := m.ExecuteEnvelope(block, delivery)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if status.Error != nil {
		return errors.UnknownError.Wrap(status.Error)
	}

	err = m.EndBlock(block)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

func (m *Executor) LoadStateRoot(batch *database.Batch) ([]byte, error) {
	_, err := batch.Account(m.Describe.NodeUrl()).GetState()
	switch {
	case err == nil:
		return batch.BptRoot(), nil
	case errors.Is(err, storage.ErrNotFound):
		return nil, nil
	default:
		return nil, errors.UnknownError.WithFormat("load partition identity: %w", err)
	}
}

func (m *Executor) RestoreSnapshot(db database.Beginner, file ioutil2.SectionReader) error {
	err := snapshot.FullRestore(db, file, m.logger, &m.Describe)
	if err != nil {
		return errors.UnknownError.WithFormat("load state: %w", err)
	}

	err = m.loadGlobals(db.View)
	if err != nil {
		return errors.InternalError.WithFormat("failed to load globals: %w", err)
	}

	return nil
}

func (x *Executor) InitChainValidators(initVal []abci.ValidatorUpdate) (additional [][]byte, err error) {
	// Verify the initial keys are ED25519 and build a map
	initValMap := map[[32]byte]bool{}
	for _, val := range initVal {
		key := val.PubKey.GetEd25519()
		if key == nil {
			return nil, errors.BadRequest.WithFormat("validator key type %T is not supported", val.PubKey.Sum)
		}
		if len(key) != ed25519.PublicKeySize {
			return nil, errors.BadRequest.WithFormat("invalid ED25519 key: want length %d, got %d", ed25519.PublicKeySize, len(key))
		}
		initValMap[*(*[32]byte)(key)] = true
	}

	// Capture any validators missing from the initial set
	for _, val := range x.globals.Active.Network.Validators {
		if !val.IsActiveOn(x.Describe.PartitionId) {
			continue
		}

		if initValMap[*(*[32]byte)(val.PublicKey)] {
			delete(initValMap, *(*[32]byte)(val.PublicKey))
		} else {
			additional = append(additional, val.PublicKey)
		}
	}

	// Verify no additional validators were introduced
	if len(initValMap) > 0 {
		return nil, errors.BadRequest.WithFormat("InitChain request includes %d validator(s) not present in genesis", len(initValMap))
	}

	return additional, nil
}
