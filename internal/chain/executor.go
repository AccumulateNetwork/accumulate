package chain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
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

	executors  map[types.TxType]TxExecutor
	dispatcher *dispatcher

	wg      *sync.WaitGroup
	mu      *sync.Mutex
	chainWG map[uint64]*sync.WaitGroup
	leader  bool
	height  int64
	dbTx    *state.DBTransaction
	time    time.Time
	logger  log.Logger
}

var _ abci.Chain = (*Executor)(nil)

type ExecutorOptions struct {
	DB      *state.StateDB
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
		var err error
		m.dispatcher, err = newDispatcher(opts)
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

	height, err := m.DB.BlockIndex()
	if errors.Is(err, storage.ErrNotFound) {
		height = 0
	} else if err != nil {
		return nil, err
	}

	m.logInfo("Loaded", "height", height, "hash", logging.AsHex(m.DB.RootHash()))
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

func (m *Executor) Genesis(time time.Time, callback func(st *StateManager) error) ([]byte, error) {
	var err error

	if !m.isGenesis {
		panic("Cannot call Genesis on a node txn executor")
	}

	m.height = 1
	m.time = time

	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = protocol.ACME
	tx.Transaction, err = new(protocol.SyntheticGenesis).MarshalBinary()
	if err != nil {
		return nil, err
	}

	m.dbTx = m.DB.Begin()
	st, err := NewStateManager(m.dbTx, tx)
	if err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	st.logger = m.logger

	txPending := state.NewPendingTransaction(tx)
	txAccepted, txPending := state.NewTransaction(txPending)
	dataAccepted, err := txAccepted.MarshalBinary()
	if err != nil {
		return nil, err
	}
	txPending.Status = json.RawMessage("{\"code\":\"0\"}")
	dataPending, err := txPending.MarshalBinary()
	if err != nil {
		return nil, err
	}

	chainId := protocol.AcmeUrl().ResourceChain32()
	err = m.dbTx.AddTransaction((*types.Bytes32)(&chainId), tx.TransactionHash(), &state.Object{Entry: dataPending}, &state.Object{Entry: dataAccepted})
	if err != nil {
		return nil, err
	}

	err = callback(st)
	if err != nil {
		return nil, err
	}

	err = st.Commit()
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
	data, err = src.Get(storage.MakeKey("BPT", "Root"))
	switch {
	case err == nil:
		bpt := new(pmt.BPT)
		bpt.Root = new(pmt.Node)
		bpt.UnMarshal(data)
		hash = bpt.Root.Hash
	case errors.Is(err, storage.ErrNotFound):
		// OK
	default:
		return fmt.Errorf("failed to load BPT root hash from app state: %v", err)
	}

	// Dump the genesis state into the key-value store
	dst := m.DB.GetDB().DB
	err = dst.EndBatch(src.Export())
	if err != nil {
		return fmt.Errorf("failed to load app state into database: %v", err)
	}

	// Load the genesis state into the StateDB
	err = m.DB.Load(dst, false)
	if err != nil {
		return fmt.Errorf("failed to reload state database: %v", err)
	}

	// Make sure the StateDB BPT root hash matches what we found in the genesis state
	if !bytes.Equal(hash[:], m.DB.RootHash()) {
		panic("BPT root hash from state DB does not match the app state")
	}

	// Make sure the subnet ID is set
	_, err = m.DB.SubnetID()
	if err != nil {
		return fmt.Errorf("failed to load subnet ID: %v", err)
	}

	return nil
}

// BeginBlock implements ./abci.Chain
func (m *Executor) BeginBlock(req abci.BeginBlockRequest) (abci.BeginBlockResponse, error) {
	m.logDebug("Begin block", "height", req.Height, "leader", req.IsLeader, "time", req.Time)

	m.leader = req.IsLeader
	m.height = req.Height
	m.time = req.Time
	m.chainWG = make(map[uint64]*sync.WaitGroup, chainWGSize)
	m.dbTx = m.DB.Begin()

	// In order for other BVCs to be able to validate the synthetic transaction,
	// a wrapped signed version must be resubmitted to this BVC network and the
	// UNSIGNED version of the transaction along with the Leader address will be
	// stored in a SynthChain in the SMT on this BVC. The BVCs will validate the
	// synth transaction against the receipt and EVERYONE will then send out the
	// wrapped TX along with the proof from the directory chain. If by end block
	// there are still unprocessed synthetic TX's the current leader takes over,
	// invalidates the previous leader's signed tx, signs the unprocessed synth
	// tx, and tries again with the new leader. By EVERYONE submitting the
	// leader signed synth tx to the designated BVC network it takes advantage
	// of the flood-fill gossip network tendermint will provide and ensure the
	// synth transaction will be picked up.

	// Reset dispatcher
	m.dispatcher.Reset(context.Background())

	// Sign synthetic transactions produced by the previous block
	err := m.signSynthTxns()
	if err != nil {
		return abci.BeginBlockResponse{}, err
	}

	// Send synthetic transactions produced in the previous block
	txns, err := m.sendSynthTxns()
	if err != nil {
		return abci.BeginBlockResponse{}, err
	}

	// Dispatch transactions. Due to Tendermint's locks, this cannot be
	// synchronous.
	go func() {
		err := m.dispatcher.Send(context.Background())
		if err != nil {
			m.logger.Error("Failed to dispatch transactions", "error", err)
		}
	}()

	return abci.BeginBlockResponse{
		SynthTxns: txns,
	}, nil
}

// EndBlock implements ./abci.Chain
func (m *Executor) EndBlock(req abci.EndBlockRequest) {}

// Commit implements ./abci.Chain
func (m *Executor) Commit() ([]byte, error) {
	m.wg.Wait()

	subnet, err := m.DB.SubnetID()
	if err != nil {
		return nil, fmt.Errorf("failed to load subnet ID: %v", err)
	}

	mdRoot, err := m.dbTx.Commit(m.height, m.time, func() error {
		// Add a synthetic transaction for the previous block's anchor
		err := m.addAnchorTxn()
		if err != nil {
			return err
		}

		// Mirror the subnet's ADI, but only immediately after genesis
		if m.height != 2 || m.IsTest {
			// TODO Don't skip during testing
			return nil
		}

		var txns []*transactions.GenTransaction
		switch m.Network.Type {
		case config.Directory:
			// Mirror DN ADI
			mirror, err := m.mirrorADIs(protocol.DnUrl())
			if err != nil {
				return fmt.Errorf("failed to mirror BVN ADI: %v", err)
			}

			for _, bvn := range m.Network.BvnNames {
				tx, err := m.buildSynthTxn(protocol.BvnUrl(bvn), mirror)
				if err != nil {
					return err
				}
				txns = append(txns, tx)
			}

		case config.BlockValidator:
			// Mirror BVN ADI
			mirror, err := m.mirrorADIs(protocol.BvnUrl(subnet))
			if err != nil {
				return fmt.Errorf("failed to mirror BVN ADI: %v", err)
			}

			tx, err := m.buildSynthTxn(protocol.DnUrl(), mirror)
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
	})
	if err != nil {
		return nil, err
	}

	m.logInfo("Committed", "db_time", m.DB.TimeBucket)
	m.DB.TimeBucket = 0
	return mdRoot, nil
}
