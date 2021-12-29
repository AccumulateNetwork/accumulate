package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
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
	blockMeta   blockMetadata
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

type blockMetadata struct {
	Deliver     DeliverMetadata
	Delivered   int
	SynthSigned int
	SynthSent   int
}

func (b *blockMetadata) Empty() bool {
	return b.Deliver.Empty() &&
		b.Delivered == 0 &&
		b.SynthSigned == 0 &&
		b.SynthSent == 0
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
		var err error
		m.governor, err = newGovernor(opts)
		if err != nil {
			return nil, err
		}
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

	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = protocol.ACME
	tx.Transaction, err = new(protocol.InternalGenesis).MarshalBinary()
	if err != nil {
		return nil, err
	}

	st, err := NewStateManager(m.blockBatch, m.Network.NodeUrl(), tx)
	if err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	st.logger.L = m.logger

	txPending := state.NewPendingTransaction(tx)
	txAccepted, txPending := state.NewTransaction(txPending)

	status := &protocol.TransactionStatus{Delivered: true}
	err = m.blockBatch.Transaction(tx.TransactionHash()).Put(txAccepted, status, nil)
	if err != nil {
		return nil, err
	}

	err = callback(st)
	if err != nil {
		return nil, err
	}

	m.blockMeta.Deliver, err = st.Commit()
	if err != nil {
		return nil, err
	}

	return m.Commit()
}

func (m *Executor) InitChain(data []byte) error {
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
	batch.Import(src)
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

	return nil
}

// BeginBlock implements ./abci.Chain
func (m *Executor) BeginBlock(req abci.BeginBlockRequest) (abci.BeginBlockResponse, error) {
	m.logDebug("Begin block", "height", req.Height, "leader", req.IsLeader, "time", req.Time)

	m.chainWG = make(map[uint64]*sync.WaitGroup, chainWGSize)
	m.blockLeader = req.IsLeader
	m.blockIndex = req.Height
	m.blockTime = req.Time
	m.blockBatch = m.DB.Begin()
	m.blockMeta = blockMetadata{}

	m.governor.DidBeginBlock(req.IsLeader)

	return abci.BeginBlockResponse{}, nil
}

// EndBlock implements ./abci.Chain
func (m *Executor) EndBlock(req abci.EndBlockRequest) {}

// Commit implements ./abci.Chain
func (m *Executor) Commit() ([]byte, error) {
	m.wg.Wait()

	// Discard changes if commit fails
	defer m.blockBatch.Discard()

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
		err := m.governor.DidCommit(m.blockBatch, m.blockLeader, m.blockIndex, m.blockTime)
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
	ledgerState.Chains = chains
	ledgerState.SystemTxns = nil
	err = ledger.PutState(ledgerState)
	if err != nil {
		return err
	}

	// Load the state of the synth list
	synthUrl := m.Network.NodeUrl().JoinPath(protocol.Synthetic)
	synth := m.blockBatch.Record(synthUrl)
	synthState := state.NewSyntheticTransactionChain()
	err = synth.GetStateAs(synthState)
	if err != nil {
		return err
	}

	// Update synth chain
	synthState.Index = m.blockIndex
	synthState.Count = int64(len(m.blockMeta.Deliver.Submitted))
	synth.PutState(synthState)

	// Produce an anchor transaction (if necessary)
	err = m.addAnchorTxn()
	if err != nil {
		return err
	}

	// Mirror the subnet's ADI, but only immediately after genesis
	if m.blockIndex != 2 || m.IsTest {
		// TODO Don't skip during testing
		return err
	}

	// Mirror subnet ADI
	mirror, err := m.mirrorADIs(m.Network.NodeUrl())
	if err != nil {
		return fmt.Errorf("failed to mirror subnet ADI: %v", err)
	}

	var txns []*transactions.GenTransaction
	switch m.Network.Type {
	case config.Directory:
		for _, bvn := range m.Network.BvnNames {
			tx, err := m.buildSynthTxn(protocol.BvnUrl(bvn), mirror, m.blockBatch)
			if err != nil {
				return err
			}
			txns = append(txns, tx)
		}

	case config.BlockValidator:
		tx, err := m.buildSynthTxn(protocol.DnUrl(), mirror, m.blockBatch)
		if err != nil {
			return fmt.Errorf("failed to build mirror txn: %v", err)
		}
		txns = append(txns, tx)
	}

	err = m.addSystemTxns(txns...)
	if err != nil {
		return fmt.Errorf("failed to save mirror txn: %v", err)
	}

	return nil
}
