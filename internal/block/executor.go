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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
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

	executors      map[protocol.TransactionType]TransactionExecutor
	dispatcher     *dispatcher
	logger         logging.OptionalLogger
	networkGlobals *protocol.NetworkGlobals // TODO update when changed

	// oldBlockMeta blockMetadata
}

type ExecutorOptions struct {
	Logger  log.Logger
	Key     ed25519.PrivateKey
	Router  routing.Router
	Network config.Network

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

	switch opts.Network.Type {
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
		return nil, fmt.Errorf("invalid subnet type %v", opts.Network.Type)
	}

	// This is a no-op in dev
	executors = addTestnetExecutors(executors)

	return newExecutor(opts, db, executors...)
}

// NewGenesisExecutor creates a transaction executor that can be used to set up
// the genesis state.
func NewGenesisExecutor(db *database.Database, logger log.Logger, network config.Network, router routing.Router) (*Executor, error) {
	return newExecutor(
		ExecutorOptions{
			Network:   network,
			Logger:    logger,
			Router:    router,
			isGenesis: true,
		},
		db,
		WriteData{},
	)
}

func newExecutor(opts ExecutorOptions, db *database.Database, executors ...TransactionExecutor) (*Executor, error) {
	m := new(Executor)
	m.ExecutorOptions = opts
	m.executors = map[protocol.TransactionType]TransactionExecutor{}
	m.dispatcher = newDispatcher(opts)

	if opts.Logger != nil {
		m.logger.L = opts.Logger.With("module", "executor")
	}

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(fmt.Errorf("duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x

	}

	batch := db.Begin(false)
	defer batch.Discard()

	var height uint64
	var ledger *protocol.SystemLedger
	err := batch.Account(m.Network.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		height = ledger.Index
	case errors.Is(err, storage.ErrNotFound):
		height = 0
	default:
		return nil, err
	}

	/*if !opts.isGenesis {
		url := opts.Network.NodeUrl(protocol.Globals)
		entry, err := indexing.Data(batch, url).GetLatestEntry()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest globals data entry: %v", err)
		}
		globals := new(protocol.NetworkGlobals)
		err = globals.UnmarshalBinary(entry.GetData()[0])
		if err != nil {
			return nil, fmt.Errorf("failed to decode latest globals entry: %v", err)
		}
		m.networkGlobals = globals
	}*/

	m.logger.Debug("Loaded", "height", height, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))
	return m, nil
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

	st := NewStateManager(&m.Network, block.Batch.Begin(true), nil, txn, m.logger.With("operation", "Genesis"))
	defer st.Discard()

	err = block.Batch.Transaction(txn.GetHash()).PutStatus(&protocol.TransactionStatus{
		Initiator: txn.Header.Principal,
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = indexing.BlockState(block.Batch, m.Network.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	_, err = m.ExecuteEnvelope(block, delivery)
	if err != nil {
		return errors.Wrap(errors.StatusUnknown, err)
	}

	err = m.EndBlock(block)
	if err != nil {
		return protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}

	return nil
}

func (m *Executor) LoadStateRoot(batch *database.Batch) ([]byte, error) {
	_, err := batch.Account(m.Network.NodeUrl()).GetState()
	switch {
	case err == nil:
		return batch.BptRoot(), nil
	case errors.Is(err, storage.ErrNotFound):
		return nil, nil
	default:
		return nil, err
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
		return fmt.Errorf("failed to unmarshal app state: %v", err)
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
		return fmt.Errorf("failed to import database: %v", err)
	}

	// Commit the database batch
	err = subbatch.Commit()
	if err != nil {
		return fmt.Errorf("failed to load app state into database: %v", err)
	}

	root := batch.BptRoot()

	// Make sure the database BPT root hash matches what we found in the genesis state
	if !bytes.Equal(srcRoot, root) {
		panic(fmt.Errorf("Root chain anchor from state DB does not match the app state\nWant: %X\nGot:  %X", srcRoot, root))
	}

	return nil
}

func (m *Executor) InitFromSnapshot(batch *database.Batch, file ioutil2.SectionReader) error {
	err := batch.RestoreSnapshot(file)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	return nil
}

func (m *Executor) SaveSnapshot(batch *database.Batch, file io.WriteSeeker) error {
	return batch.SaveSnapshot(file, &m.Network)
}
