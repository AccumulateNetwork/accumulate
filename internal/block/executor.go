package block

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

type Executor struct {
	ExecutorOptions

	globals    *Globals
	executors  map[protocol.TransactionType]TransactionExecutor
	dispatcher *dispatcher
	logger     logging.OptionalLogger
	db         *database.Database

	// oldBlockMeta blockMetadata
}

type ExecutorOptions struct {
	Logger   log.Logger
	Key      ed25519.PrivateKey
	Router   routing.Router
	Describe config.Describe
	EventBus *events.Bus

	isGenesis bool
}

// NewNodeExecutor creates a new Executor for a node.
func NewNodeExecutor(opts ExecutorOptions, db *database.Database) (*Executor, error) {
	executors := []TransactionExecutor{
		// User transactions
		AddCredits{},
		BurnTokens{},
		CreateDataAccount{},
		CreateIdentity{},
		CreateKeyBook{},
		CreateKeyPage{},
		CreateToken{},
		CreateTokenAccount{},
		IssueTokens{},
		SendTokens{},
		UpdateKeyPage{},
		WriteData{},
		WriteDataTo{},
		UpdateAccountAuth{},
		UpdateKey{},

		// Synthetic
		SyntheticBurnTokens{},
		SyntheticCreateIdentity{},
		SyntheticDepositCredits{},
		SyntheticDepositTokens{},
		SyntheticWriteData{},

		// Forwarding
		SyntheticForwardTransaction{},

		// Validator management
		AddValidator{},
		RemoveValidator{},
		UpdateValidatorKey{},
	}

	switch opts.Describe.NetworkType {
	case config.Directory:
		executors = append(executors,
			PartitionAnchor{},
			DirectoryAnchor{},
		)

	case config.BlockValidator:
		executors = append(executors,
			DirectoryAnchor{},
		)

	default:
		return nil, errors.Format(errors.StatusInternalError, "invalid subnet type %v", opts.Describe.NetworkType)
	}

	// This is a no-op in dev
	executors = addTestnetExecutors(executors)

	return newExecutor(opts, db, executors...)
}

// NewGenesisExecutor creates a transaction executor that can be used to set up
// the genesis state.
func NewGenesisExecutor(db *database.Database, logger log.Logger, network *config.Describe, router routing.Router) (*Executor, error) {
	return newExecutor(
		ExecutorOptions{
			Describe:  *network,
			Logger:    logger,
			Router:    router,
			isGenesis: true,
		},
		db,
		SystemWriteData{},
	)
}

func newExecutor(opts ExecutorOptions, db *database.Database, executors ...TransactionExecutor) (*Executor, error) {
	m := new(Executor)
	m.ExecutorOptions = opts
	m.executors = map[protocol.TransactionType]TransactionExecutor{}
	m.dispatcher = newDispatcher(opts)
	m.db = db

	if opts.Logger != nil {
		m.logger.L = opts.Logger.With("module", "executor")
	}

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(errors.Format(errors.StatusInternalError, "duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x
	}

	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err := batch.Account(m.Describe.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		// Database has been initialized
		m.logger.Debug("Loaded", "height", ledger.Index, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))

		// Load globals
		err = m.loadGlobals(db.View)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}

	case errors.Is(err, storage.ErrNotFound):
		// Database is uninitialized
		m.logger.Debug("Loaded", "height", 0, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))

	default:
		return nil, errors.Format(errors.StatusUnknown, "load ledger: %w", err)
	}

	return m, nil
}

func (m *Executor) ActiveGlobals_TESTONLY() *core.GlobalValues {
	return &m.globals.Active
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

	st := NewStateManager(&m.Describe, nil, block.Batch.Begin(true), nil, txn, m.logger.With("operation", "Genesis"))
	defer st.Discard()

	err = block.Batch.Transaction(txn.GetHash()).PutStatus(&protocol.TransactionStatus{
		Initiator: txn.Header.Principal,
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = indexing.BlockState(block.Batch, m.Describe.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	status, err := m.ExecuteEnvelope(block, delivery)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	if status.Code != 0 {
		if status.Error != nil {
			return errors.Wrap(errors.StatusUnknown, status.Error)
		}
		return errors.New(errors.StatusUnknown, status.Message)
	}

	err = m.EndBlock(block)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
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
		return nil, errors.Format(errors.StatusUnknown, "load subnet identity: %w", err)
	}
}

func (m *Executor) InitFromGenesis(batch *database.Batch, data []byte) error {
	if m.isGenesis {
		panic("Cannot call InitChain on a genesis txn executor")
	}

	// Load the genesis state (JSON) into an in-memory key-value store
	src := memory.New(nil)
	err := src.UnmarshalJSON(data)
	if err != nil {
		return errors.Format(errors.StatusInternalError, "failed to unmarshal app state: %v", err)
	}

	// Load the root anchor chain so we can verify the system state
	srcBatch := database.New(src, nil).Begin(false)
	defer srcBatch.Discard()
	srcRoot := srcBatch.BptRoot()

	// Dump the genesis state into the key-value store
	subbatch := batch.Begin(true)
	defer subbatch.Discard()
	err = subbatch.Import(src)
	if err != nil {
		return errors.Format(errors.StatusInternalError, "failed to import database: %v", err)
	}

	// Commit the database batch
	err = subbatch.Commit()
	if err != nil {
		return errors.Format(errors.StatusInternalError, "failed to load app state into database: %v", err)
	}

	root := batch.BptRoot()

	// Make sure the database BPT root hash matches what we found in the genesis state
	if !bytes.Equal(srcRoot, root) {
		panic(errors.Format(errors.StatusInternalError, "Root chain anchor from state DB does not match the app state\nWant: %X\nGot:  %X", srcRoot, root))
	}

	err = m.loadGlobals(batch.View)
	if err != nil {
		return fmt.Errorf("failed to load globals: %v", err)
	}

	return nil
}

func (m *Executor) InitFromSnapshot(batch *database.Batch, file ioutil2.SectionReader) error {
	err := batch.RestoreSnapshot(file)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load state: %w", err)
	}

	err = m.loadGlobals(batch.View)
	if err != nil {
		return fmt.Errorf("failed to load globals: %v", err)
	}

	return nil
}

func (m *Executor) SaveSnapshot(batch *database.Batch, file io.WriteSeeker) error {
	return batch.SaveSnapshot(file, &m.Describe)
}
