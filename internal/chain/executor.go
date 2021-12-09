package chain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
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
	isGenesis  bool

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
	DB              *state.StateDB
	Logger          log.Logger
	Key             ed25519.PrivateKey
	Local           api.ABCIBroadcastClient
	SubnetType      config.NetworkType
	Directory       string
	BlockValidators []string

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

	var err error
	m.dispatcher, err = newDispatcher(opts)
	if err != nil {
		return nil, err
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

func (m *Executor) Genesis(time time.Time, callback func(st *StateManager)) ([]byte, error) {
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

	callback(st)

	err = st.Commit()
	if err != nil {
		return nil, err
	}

	return m.Commit()
}

func (m *Executor) InitChain(state []byte) error {
	if m.isGenesis {
		panic("Cannot call InitChain on a genesis txn executor")
	}

	// Load the genesis state (JSON) into an in-memory key-value store
	src := new(memory.DB)
	_ = src.InitDB("", nil)
	err := src.UnmarshalJSON(state)
	if err != nil {
		return fmt.Errorf("failed to unmarshal app state: %v", err)
	}

	// Load the BPT root hash so we can verify the system state
	var hash [32]byte
	data, err := src.Get(storage.ComputeKey("BPT", "Root"))
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
		return fmt.Errorf("faild to reload state database: %v", err)
	}

	// Make sure the StateDB BPT root hash matches what we found in the genesis state
	if !bytes.Equal(hash[:], m.DB.RootHash()) {
		panic("BPT root hash from state DB does not match the app state")
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

func (m *Executor) check(tx *transactions.GenTransaction) (*StateManager, error) {
	if len(tx.Signature) == 0 {
		return nil, fmt.Errorf("transaction is not signed")
	}

	if !tx.ValidateSig() {
		return nil, fmt.Errorf("invalid signature")
	}

	txt := tx.TransactionType()

	if m.dbTx == nil {
		m.dbTx = m.DB.Begin()
	}
	st, err := NewStateManager(m.dbTx, tx)
	if errors.Is(err, storage.ErrNotFound) {
		switch txt {
		case types.TxTypeSyntheticCreateChain, types.TxTypeSyntheticDepositTokens:
			// TX does not require a sponsor - it may create the sponsor
		default:
			return nil, fmt.Errorf("sponsor not found: %v", err)
		}
	} else if err != nil {
		return nil, err
	}

	if txt.IsSynthetic() {
		return st, m.checkSynthetic(st, tx)
	}

	book := new(protocol.KeyBook)
	switch sponsor := st.Sponsor.(type) {
	case *protocol.LiteTokenAccount:
		return st, m.checkLite(st, tx, sponsor)

	case *state.AdiState, *state.TokenAccount, *protocol.KeyPage, *protocol.DataAccount:
		if (sponsor.Header().KeyBook == types.Bytes32{}) {
			return nil, fmt.Errorf("sponsor has not been assigned to an SSG")
		}
		err := st.LoadAs(sponsor.Header().KeyBook, book)
		if err != nil {
			return nil, fmt.Errorf("invalid KeyBook: %v", err)
		}

	case *protocol.KeyBook:
		book = sponsor

	default:
		// The TX sponsor cannot be a transaction
		// Token issue chains are not implemented
		return nil, fmt.Errorf("invalid sponsor: chain type %v cannot sponsor transactions", sponsor.Header().Type)
	}

	if tx.SigInfo.KeyPageIndex >= uint64(len(book.Pages)) {
		return nil, fmt.Errorf("invalid sig spec index")
	}

	page := new(protocol.KeyPage)
	err = st.LoadAs(book.Pages[tx.SigInfo.KeyPageIndex], page)
	if err != nil {
		return nil, fmt.Errorf("invalid sig spec: %v", err)
	}

	// TODO check height
	height, err := st.GetHeight(book.Pages[tx.SigInfo.KeyPageIndex])
	if err != nil {
		return nil, err
	}
	if height != tx.SigInfo.KeyPageHeight {
		return nil, fmt.Errorf("invalid height")
	}

	for i, sig := range tx.Signature {
		ks := page.FindKey(sig.PublicKey)
		if ks == nil {
			return nil, fmt.Errorf("no key spec matches signature %d", i)
		}

		switch {
		case i > 0:
			// Only check the nonce of the first key
		case ks.Nonce >= sig.Nonce:
			return nil, fmt.Errorf("invalid nonce")
		default:
			ks.Nonce = sig.Nonce
		}
	}

	//cost, err := protocol.ComputeFee(tx)
	//if err != nil {
	//	return nil, err
	//}
	//if !page.DebitCredits(uint64(cost)) {
	//	return nil, fmt.Errorf("insufficent credits for the transaction")
	//}
	//
	//st.UpdateCreditBalance(page)
	st.UpdateNonce(page)
	return st, nil
}

func (m *Executor) checkSynthetic(st *StateManager, tx *transactions.GenTransaction) error {
	//placeholder for special validation rules for synthetic transactions.
	//need to verify the sender is a legit bvc validator also need the dbvc receipt
	//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
	//not an impostor. Need to figure out how to do this. Right now we just assume the synth request
	//sender is legit.
	return nil
}

func (m *Executor) checkLite(st *StateManager, tx *transactions.GenTransaction, account *protocol.LiteTokenAccount) error {
	u, err := account.ParseUrl()
	if err != nil {
		// This shouldn't happen because invalid URLs should never make it
		// into the database.
		return fmt.Errorf("invalid sponsor URL: %v", err)
	}

	urlKH, _, err := protocol.ParseLiteAddress(u)
	if err != nil {
		// This shouldn't happen because invalid URLs should never make it
		// into the database.
		return fmt.Errorf("invalid lite token URL: %v", err)
	}

	for i, sig := range tx.Signature {
		sigKH := sha256.Sum256(sig.PublicKey)
		if !bytes.Equal(urlKH, sigKH[:20]) {
			return fmt.Errorf("signature %d's public key does not match the sponsor", i)
		}

		switch {
		case i > 0:
			// Only check the nonce of the first key
		case account.Nonce >= sig.Nonce:
			return fmt.Errorf("invalid nonce")
		default:
			account.Nonce = sig.Nonce
		}
	}

	st.UpdateNonce(account)
	return nil
}

// CheckTx implements ./abci.Chain
func (m *Executor) CheckTx(tx *transactions.GenTransaction) *protocol.Error {
	err := tx.SetRoutingChainID()
	if err != nil {
		return &protocol.Error{Code: protocol.CodeRoutingChainId, Message: err}
	}

	st, err := m.check(tx)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeCheckTxError, Message: err}
	}

	executor, ok := m.executors[types.TxType(tx.TransactionType())]
	if !ok {
		return &protocol.Error{Code: protocol.CodeInvalidTxnType, Message: fmt.Errorf("unsupported TX type: %v", types.TxType(tx.TransactionType()))}
	}
	err = executor.Validate(st, tx)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeValidateTxnError, Message: err}
	}
	return nil
}

func (m *Executor) recordTransactionError(txPending *state.PendingTransaction, chainId *types.Bytes32, txid []byte, err *protocol.Error) *protocol.Error {
	txPending.Status = json.RawMessage(fmt.Sprintf("{\"code\":\"1\", \"error\":\"%v\"}", err))
	txPendingObject := new(state.Object)
	e, err1 := txPending.MarshalBinary()
	txPendingObject.Entry = e
	if err1 != nil {
		return &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("failed marshaling pending tx (%v) on error: %v", err1, err)}
	}
	err1 = m.dbTx.AddTransaction(chainId, txid, txPendingObject, nil)
	if err1 != nil {
		err = &protocol.Error{Code: protocol.CodeAddTxnError, Message: fmt.Errorf("error adding pending tx (%v) on error %v", err1, err)}
	}
	return err
}

// DeliverTx implements ./abci.Chain
func (m *Executor) DeliverTx(tx *transactions.GenTransaction) *protocol.Error {
	m.wg.Add(1)

	// If this is done async (`go m.deliverTxAsync(tx)`), how would an error
	// get back to the ABCI callback?
	// > errors would not go back to ABCI callback. The errors determine what gets kept in tm TX history, so as far
	// > as tendermint is concerned, it will keep everything, but we don't care because we are pruning tm history.
	// > For reporting errors back to the world, we would to provide a different mechanism for querying
	// > tx status, which can be done via the pending chains.  Thus, going on that assumption, because each
	// > identity operates independently, we can make the validation process highly parallel, and sync up at
	// > the commit when we go to write the states.
	// go func() {

	defer m.wg.Done()

	if tx.Transaction == nil || tx.SigInfo == nil || len(tx.ChainID) != 32 {
		return &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: fmt.Errorf("malformed transaction error")}
	}

	txt := tx.TransactionType()
	executor, ok := m.executors[txt]
	txPending := state.NewPendingTransaction(tx)
	chainId := types.Bytes(tx.ChainID).AsBytes32()
	if !ok {
		return m.recordTransactionError(txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeInvalidTxnType, Message: fmt.Errorf("unsupported TX type: %v", tx.TransactionType().Name())})
	}

	tx.TransactionHash()

	m.mu.Lock()
	group, ok := m.chainWG[tx.Routing%chainWGSize]
	if !ok {
		group = new(sync.WaitGroup)
		m.chainWG[tx.Routing%chainWGSize] = group
	}

	group.Wait()
	group.Add(1)
	defer group.Done()
	m.mu.Unlock()

	st, err := m.check(tx)
	if err != nil {
		return m.recordTransactionError(txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeCheckTxError, Message: fmt.Errorf("txn check failed : %v", err)})
	}

	// Validate
	// TODO result should return a list of chainId's the transaction touched.
	err = executor.Validate(st, tx)
	if err != nil {
		return m.recordTransactionError(txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: fmt.Errorf("txn validation failed : %v", err)})
	}

	// If we get here, we were successful in validating.  So, we need to
	// split the transaction in 2, the body (i.e. TxAccepted), and the
	// validation material (i.e. TxPending).  The body of the transaction
	// gets put on the main chain, and the validation material gets put on
	// the pending chain which is purged after about 2 weeks
	txAccepted, txPending := state.NewTransaction(txPending)
	txAcceptedObject := new(state.Object)
	txAcceptedObject.Entry, err = txAccepted.MarshalBinary()
	if err != nil {
		return m.recordTransactionError(txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeMarshallingError, Message: err})
	}

	txPendingObject := new(state.Object)
	txPending.Status = json.RawMessage(fmt.Sprintf("{\"code\":\"0\"}"))
	txPendingObject.Entry, err = txPending.MarshalBinary()
	if err != nil {
		return m.recordTransactionError(txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeMarshallingError, Message: err})
	}

	// Store the tx state
	err = m.dbTx.AddTransaction(&chainId, tx.TransactionHash(), txPendingObject, txAcceptedObject)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeTxnStateError, Message: err}
	}

	// Store pending state updates, queue state creates for synthetic transactions
	err = st.Commit()
	if err != nil {
		return m.recordTransactionError(txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeRecordTxnError, Message: err})
	}

	// Process synthetic transactions generated by the validator
	err = m.addSynthTxns(tx.TransactionHash(), st)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeSyntheticTxnError, Message: err}
	}

	return nil
}

// EndBlock implements ./abci.Chain
func (m *Executor) EndBlock(req abci.EndBlockRequest) {}

// Commit implements ./abci.Chain
func (m *Executor) Commit() ([]byte, error) {
	m.wg.Wait()

	mdRoot, err := m.dbTx.Commit(m.height, m.time, func() error {
		// Add a synthetic transaction for the previous block's anchor
		return m.addAnchorTxn()
	})
	if err != nil {
		return nil, err
	}

	m.logInfo("Committed", "db_time", m.DB.TimeBucket)
	m.DB.TimeBucket = 0
	return mdRoot, nil
}
