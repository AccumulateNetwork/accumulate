// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var messageExecutors []func(ExecutorOptions) (messaging.MessageType, MessageExecutor)
var signatureExecutors []func(ExecutorOptions) (protocol.SignatureType, SignatureExecutor)

type Executor struct {
	ExecutorOptions
	BlockTimers TimerSet

	globals            *Globals
	executors          map[protocol.TransactionType]chain.TransactionExecutor
	messageExecutors   map[messaging.MessageType]MessageExecutor
	signatureExecutors map[protocol.SignatureType]SignatureExecutor
	logger             logging.OptionalLogger
	db                 database.Beginner
	isValidator        bool
	isGenesis          bool
	mainDispatcher     Dispatcher
}

type ExecutorOptions = execute.Options
type Dispatcher = execute.Dispatcher

// NewExecutor creates a new Executor.
func NewExecutor(opts ExecutorOptions) (*Executor, error) {
	txnX := []chain.TransactionExecutor{
		// User transactions
		chain.AddCredits{},
		chain.BurnCredits{},
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
		chain.TransferCredits{},
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

		// Operator transactions
		chain.ActivateProtocolVersion{},
	}

	switch opts.Describe.NetworkType {
	case protocol.PartitionTypeDirectory:
		txnX = append(txnX,
			chain.PartitionAnchor{},
			chain.DirectoryAnchor{},
		)

	case protocol.PartitionTypeBlockValidator:
		txnX = append(txnX,
			chain.DirectoryAnchor{},
		)

	default:
		return nil, errors.InternalError.WithFormat("invalid partition type %v", opts.Describe.NetworkType)
	}

	if opts.BackgroundTaskLauncher == nil {
		opts.BackgroundTaskLauncher = func(f func()) { go f() }
	}

	m := new(Executor)
	m.ExecutorOptions = opts
	m.executors = map[protocol.TransactionType]chain.TransactionExecutor{}
	m.messageExecutors = newExecutorMap(opts, messageExecutors)
	m.signatureExecutors = newExecutorMap(opts, signatureExecutors)
	m.db = opts.Database
	m.mainDispatcher = opts.NewDispatcher()
	m.isGenesis = false

	m.db.SetObserver(internal.NewDatabaseObserver())

	if opts.Logger != nil {
		m.logger.L = opts.Logger.With("module", "executor")
	}

	for _, x := range txnX {
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

func (x *Executor) LastBlock() (*execute.BlockParams, [32]byte, error) {
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

	b := new(execute.BlockParams)
	b.Index = entry.BlockIndex
	b.Time = *entry.BlockTime

	return b, *(*[32]byte)(batch.BptRoot()), nil
}

func (x *Executor) Restore(file ioutil2.SectionReader, validators []*execute.ValidatorUpdate) (additional []*execute.ValidatorUpdate, err error) {
	batch := x.Database.Begin(true)
	defer batch.Discard()

	err = snapshot.FullRestore(x.Database, file, x.logger, &x.Describe)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state: %w", err)
	}

	err = x.loadGlobals(x.Database.View)
	if err != nil {
		return nil, errors.InternalError.WithFormat("failed to load globals: %w", err)
	}

	// Verify the initial keys are ED25519 and build a map
	initValMap := map[[32]byte]bool{}
	for _, val := range validators {
		if val.Type != protocol.SignatureTypeED25519 {
			return nil, errors.BadRequest.WithFormat("validator key type %T is not supported", val.Type)
		}
		if len(val.PublicKey) != ed25519.PublicKeySize {
			return nil, errors.BadRequest.WithFormat("invalid ED25519 key: want length %d, got %d", ed25519.PublicKeySize, len(val.PublicKey))
		}
		initValMap[*(*[32]byte)(val.PublicKey)] = true
	}

	// Capture any validators missing from the initial set
	for _, val := range x.globals.Active.Network.Validators {
		if !val.IsActiveOn(x.Describe.PartitionId) {
			continue
		}

		if initValMap[*(*[32]byte)(val.PublicKey)] {
			delete(initValMap, *(*[32]byte)(val.PublicKey))
		} else {
			additional = append(additional, &execute.ValidatorUpdate{
				Type:      protocol.SignatureTypeED25519,
				PublicKey: val.PublicKey,
				Power:     1,
			})
		}
	}

	// Verify no additional validators were introduced
	if len(initValMap) > 0 {
		return nil, errors.BadRequest.WithFormat("InitChain request includes %d validator(s) not present in genesis", len(initValMap))
	}

	return additional, nil
}
