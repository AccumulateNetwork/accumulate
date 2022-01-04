package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/pmt"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/memory"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/tendermint/tendermint/libs/log"
)

const chainWGSize = 4

type Executor struct {
	ExecutorOptions

	executors map[types.TxType]TxExecutor
	governor  *governor
	logger    log.Logger

	wg      *sync.WaitGroup
	mu      *sync.Mutex
	chainWG map[uint64]*sync.WaitGroup

	blockLeader bool
	blockIndex  int64
	blockTime   time.Time
	blockBatch  *database.Batch
	blockMeta   BlockMetadata
}

var _ abci.Chain = (*Executor)(nil)

type ExecutorOptions struct {
	DB      *database.Database
	Logger  log.Logger
	Key     ed25519.PrivateKey
	Local   api.ABCIBroadcastClient
	Network config.Network

	isGenesis bool

	// TODO Remove once tests support running the DN
	IsTest bool
}

func newExecutor(opts ExecutorOptions, executors ...TxExecutor) (*Executor, error) {
	m := new(Executor)
	m.ExecutorOptions = opts
	m.executors = map[types.TxType]TxExecutor{}
	m.wg = new(sync.WaitGroup)
	m.mu = new(sync.Mutex)

	if opts.Logger != nil {
		m.logger = opts.Logger.With("module", "executor")
	}

	if !m.isGenesis {
		m.governor = newGovernor(opts)
	}

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(fmt.Errorf("duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x
	}

	batch := m.DB.Begin()
	defer batch.Discard()

	var height int64
	ledger := protocol.NewInternalLedger()
	err := batch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger)).GetStateAs(ledger)
	switch {
	case err == nil:
		height = ledger.Index
	case errors.Is(err, storage.ErrNotFound):
		height = 0
	default:
		return nil, err
	}

	m.logInfo("Loaded", "height", height, "hash", logging.AsHex(batch.RootHash()))
	return m, nil
}

func (m *Executor) logDebug(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Debug(msg, keyVals...)
	}
}

func (m *Executor) logInfo(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Info(msg, keyVals...)
	}
}

func (m *Executor) logError(msg string, keyVals ...interface{}) {
	if m.logger != nil {
		m.logger.Error(msg, keyVals...)
	}
}

func (m *Executor) Start() error {
	return m.governor.Start()
}

func (m *Executor) Stop() error {
	return m.governor.Stop()
}

func (m *Executor) Genesis(time time.Time, callback func(st *StateManager) error) ([]byte, error) {
	var err error

	if !m.isGenesis {
		panic("Cannot call Genesis on a node txn executor")
	}

	m.blockIndex = 1
	m.blockTime = time
	m.blockBatch = m.DB.Begin()

	env := new(transactions.Envelope)
	env.Transaction = new(transactions.Transaction)
	env.Transaction.Origin = protocol.AcmeUrl()
	env.Transaction.Body, err = new(protocol.InternalGenesis).MarshalBinary()
	if err != nil {
		return nil, err
	}

	st, err := NewStateManager(m.blockBatch, m.Network.NodeUrl(), env)
	if err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	st.logger.L = m.logger

	txPending := state.NewPendingTransaction(env)
	txAccepted, txPending := state.NewTransaction(txPending)

	status := &protocol.TransactionStatus{Delivered: true}
	err = m.blockBatch.Transaction(env.Transaction.Hash()).Put(txAccepted, status, nil)
	if err != nil {
		return nil, err
	}

	err = callback(st)
	if err != nil {
		return nil, err
	}

	m.blockMeta.Delivered = 1
	m.blockMeta.Deliver, err = st.Commit()
	if err != nil {
		return nil, err
	}

	ledger := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger))
	ledger.Index("Genesis", "BlockMetadata").PutAs(&m.blockMeta)
	if err != nil {
		return nil, err
	}

	return m.Commit()
}

func (m *Executor) InitChain(data []byte, time time.Time, blockIndex int64) error {
	if m.isGenesis {
		panic("Cannot call InitChain on a genesis txn executor")
	}

	// Load the genesis state (JSON) into an in-memory key-value store
	src := new(memory.DB)
	_ = src.InitDB("", nil)
	err := src.UnmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal app state: %v", err)
	}

	// Load the BPT root hash so we can verify the system state
	var hash [32]byte
	data, err = src.Begin().Get(storage.MakeKey("BPT", "Root"))
	switch {
	case err == nil:
		bpt := pmt.NewBPT()
		bpt.UnMarshal(data)
		hash = bpt.Root.Hash
	case errors.Is(err, storage.ErrNotFound):
		// OK
	default:
		return fmt.Errorf("failed to load BPT root hash from app state: %v", err)
	}

	// Dump the genesis state into the key-value store
	batch := m.DB.Begin()
	defer batch.Discard()
	batch.Import(src)

	// Load the genesis block metadata
	blockMeta := new(BlockMetadata)
	ledger := batch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger))
	err = ledger.Index("Genesis", "BlockMetadata").GetAs(blockMeta)
	if err != nil {
		return fmt.Errorf("failed to load genesis block metadata: %v", err)
	}

	// Commit the database batch
	err = batch.Commit()
	if err != nil {
		return fmt.Errorf("failed to load app state into database: %v", err)
	}

	// Recreate the batch to reload the BPT
	batch = m.DB.Begin()
	defer batch.Discard()

	// Make sure the database BPT root hash matches what we found in the genesis state
	if !bytes.Equal(hash[:], batch.RootHash()) {
		panic(fmt.Errorf("BPT root hash from state DB does not match the app state\nWant: %X\nGot:  %X", hash[:], batch.RootHash()))
	}

	return m.governor.DidCommit(batch, true, true, blockIndex, time, blockMeta)
}

// BeginBlock implements ./abci.Chain
func (m *Executor) BeginBlock(req abci.BeginBlockRequest) (abci.BeginBlockResponse, error) {
	m.logDebug("Begin block", "height", req.Height, "leader", req.IsLeader, "time", req.Time)

	m.chainWG = make(map[uint64]*sync.WaitGroup, chainWGSize)
	m.blockLeader = req.IsLeader
	m.blockIndex = req.Height
	m.blockTime = req.Time
	m.blockBatch = m.DB.Begin()
	m.blockMeta = BlockMetadata{}

	m.governor.DidBeginBlock(req.IsLeader, req.Height, req.Time)

	return abci.BeginBlockResponse{}, nil
}

// EndBlock implements ./abci.Chain
func (m *Executor) EndBlock(req abci.EndBlockRequest) {}

// Commit implements ./abci.Chain
func (m *Executor) Commit() ([]byte, error) {
	m.wg.Wait()

	// Discard changes if commit fails
	defer m.blockBatch.Discard()

	updatedMap := make(map[string]bool, len(m.blockMeta.Deliver.Updated))
	updatedSlice := make([]*url.URL, 0, len(m.blockMeta.Deliver.Updated))
	for _, u := range m.blockMeta.Deliver.Updated {
		s := strings.ToLower(u.String())
		if updatedMap[s] {
			continue
		}

		updatedSlice = append(updatedSlice, u)
		updatedMap[s] = true
	}
	m.blockMeta.Deliver.Updated = updatedSlice

	if m.blockMeta.Empty() {
		m.logInfo("Committed empty transaction")
	} else {
		m.logInfo("Committing", "height", m.blockIndex, "delivered", m.blockMeta.Delivered, "signed", m.blockMeta.SynthSigned, "sent", m.blockMeta.SynthSent, "updated", len(m.blockMeta.Deliver.Updated), "submitted", len(m.blockMeta.Deliver.Submitted))
		t := time.Now()

		err := m.doCommit()
		if err != nil {
			return nil, err
		}

		err = m.blockBatch.Commit()
		if err != nil {
			return nil, err
		}

		m.logInfo("Committed", "height", m.blockIndex, "duration", time.Since(t))
	}

	if !m.isGenesis {
		err := m.governor.DidCommit(m.blockBatch, m.blockLeader, false, m.blockIndex, m.blockTime, &m.blockMeta)
		if err != nil {
			return nil, err
		}
	}

	// Get BPT root from a clean batch
	batch := m.DB.Begin()
	defer batch.Discard()
	return batch.RootHash(), nil
}

func (m *Executor) doCommit() error {
	ledger := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Ledger))

	// Load the state of minor root
	ledgerState := protocol.NewInternalLedger()
	err := ledger.GetStateAs(ledgerState)
	switch {
	case err == nil:
		// Make sure the block index is increasing
		if ledgerState.Index >= m.blockIndex {
			panic(fmt.Errorf("Current height is %d but the next block height is %d!", ledgerState.Index, m.blockIndex))
		}

	case m.isGenesis && errors.Is(err, storage.ErrNotFound):
		// OK

	default:
		return err
	}

	// Load the main chain of the minor root
	rootChain, err := ledger.Chain(protocol.MinorRootChain)
	if err != nil {
		return err
	}

	// Add an anchor to the root chain for every updated record
	chains := make([][32]byte, 0, len(m.blockMeta.Deliver.Updated))
	for _, u := range m.blockMeta.Deliver.Updated {
		chains = append(chains, u.ResourceChain32())
		recordChain, err := m.blockBatch.Record(u).Chain(protocol.MainChain)
		if err != nil {
			return err
		}

		err = rootChain.AddEntry(recordChain.Anchor())
		if err != nil {
			return err
		}

		m.logDebug("Updated a chain", "url", u.String(), "id", logging.AsHex(u.ResourceChain()))
	}

	// Update the root state
	ledgerState.Index = m.blockIndex
	ledgerState.Timestamp = m.blockTime
	ledgerState.Records.Chains = chains
	ledgerState.Synthetic.System = nil
	ledgerState.Synthetic.Produced = uint64(len(m.blockMeta.Deliver.Submitted))
	return ledger.PutState(ledgerState)
}
