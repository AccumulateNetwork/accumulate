package block

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

type Executor struct {
	ExecutorOptions

	executors  map[protocol.TransactionType]TransactionExecutor
	dispatcher *dispatcher
	logger     logging.OptionalLogger

	// oldBlockMeta blockMetadata
}

type ExecutorOptions struct {
	Logger  log.Logger
	Key     ed25519.PrivateKey
	Router  routing.Router
	Network config.Network

	isGenesis bool
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

	var height int64
	var ledger *protocol.InternalLedger
	err := batch.Account(m.Network.NodeUrl(protocol.Ledger)).GetStateAs(&ledger)
	switch {
	case err == nil:
		height = ledger.Index
	case errors.Is(err, storage.ErrNotFound):
		height = 0
	default:
		return nil, err
	}

	m.logInfo("Loaded", "height", height, "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))
	return m, nil
}

func (m *Executor) logDebug(msg string, keyVals ...interface{}) {
	m.logger.Debug(msg, keyVals...)
}

func (m *Executor) logInfo(msg string, keyVals ...interface{}) {
	m.logger.Info(msg, keyVals...)
}

func (m *Executor) logError(msg string, keyVals ...interface{}) {
	m.logger.Error(msg, keyVals...)
}

func (m *Executor) Genesis(block *Block, callback func(st *StateManager) error) error {
	var err error

	if !m.isGenesis {
		panic("Cannot call Genesis on a node txn executor")
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AcmeUrl()
	txn.Body = new(protocol.InternalGenesis)

	st := NewStateManager(&m.Network, block.Batch.Begin(true), nil, txn, m.logger.With("operation", "Genesis"))
	defer st.Discard()

	err = putSyntheticTransaction(
		block.Batch, txn,
		&protocol.TransactionStatus{Delivered: true},
		nil)
	if err != nil {
		return err
	}

	err = indexing.BlockState(block.Batch, m.Network.NodeUrl(protocol.Ledger)).Clear()
	if err != nil {
		return err
	}

	err = callback(st)
	if err != nil {
		return err
	}

	state, err := st.Commit()
	if err != nil {
		return err
	}

	mirror, err := m.buildMirror(block.Batch)
	if err != nil {
		return err
	}

	switch m.Network.Type {
	case config.Directory:
		for _, bvn := range m.Network.GetBvnNames() {
			st.Submit(protocol.SubnetUrl(bvn), mirror)
		}

	case config.BlockValidator:
		st.Submit(protocol.DnUrl(), mirror)
	}

	block.State.MergeTransaction(state)

	err = m.ProduceSynthetic(block.Batch, txn, state.ProducedTxns)
	if err != nil {
		return protocol.NewError(protocol.ErrorCodeUnknownError, err)
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

func (m *Executor) InitFromSnapshot(batch *database.Batch, filename string) error {
	err := batch.LoadState(filename)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	return nil
}

func (m *Executor) SaveSnapshot(batch *database.Batch, filename string) error {
	return batch.SaveState(filename, &m.Network)
}

func (x *Executor) buildMirror(batch *database.Batch) (*protocol.SyntheticMirror, error) {
	mirror := new(protocol.SyntheticMirror)

	nodeUrl := x.Network.NodeUrl()
	rec, err := mirrorRecord(batch, nodeUrl)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load %s: %w", nodeUrl, err)
	}
	mirror.Objects = append(mirror.Objects, rec)

	md, err := loadDirectoryMetadata(batch, nodeUrl)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load %s directory: %w", nodeUrl, err)
	}

	for i := uint64(0); i < md.Count; i++ {
		s, err := loadDirectoryEntry(batch, nodeUrl, i)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "load %s directory entry %d: %w", nodeUrl, i, err)
		}

		u, err := url.Parse(s)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "invalid %s directory entry %d: %w", nodeUrl, i, err)
		}

		rec, err := mirrorRecord(batch, u)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "load %s: %w", u, err)
		}

		// Only mirror keys
		switch rec.Account.(type) {
		case *protocol.ADI,
			*protocol.KeyBook,
			*protocol.KeyPage:
			// Keep
		default:
			// Discard
			continue
		}

		mirror.Objects = append(mirror.Objects, rec)
	}

	return mirror, nil
}
