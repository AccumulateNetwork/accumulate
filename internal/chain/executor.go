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

	"github.com/AccumulateNetwork/accumulate/internal/abci"
	accapi "github.com/AccumulateNetwork/accumulate/internal/api"
	apiv2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/memory"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/tendermint/tendermint/libs/log"
)

const chainWGSize = 4

type Executor struct {
	db        *state.StateDB
	key       ed25519.PrivateKey
	query     *accapi.Query
	local     apiv2.ABCIBroadcastClient
	executors map[types.TxType]TxExecutor

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
	Query  *accapi.Query
	DB     *state.StateDB
	Logger log.Logger
	Key    ed25519.PrivateKey
	Local  apiv2.ABCIBroadcastClient
}

func NewExecutor(opts ExecutorOptions, executors ...TxExecutor) (*Executor, error) {
	m := new(Executor)
	m.db = opts.DB
	m.executors = map[types.TxType]TxExecutor{}
	m.key = opts.Key
	m.wg = new(sync.WaitGroup)
	m.mu = new(sync.Mutex)
	m.query = opts.Query
	m.logger = opts.Logger.With("module", "executor")
	m.local = opts.Local

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(fmt.Errorf("duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x
	}

	height, err := m.db.BlockIndex()
	if errors.Is(err, storage.ErrNotFound) {
		height = 0
	} else if err != nil {
		return nil, err
	}

	m.logger.Info("Loaded", "height", height, "hash", logging.AsHex(m.db.RootHash()))
	return m, nil
}

func (m *Executor) InitChain(state []byte) error {
	src := new(memory.DB)
	_ = src.InitDB("", nil)
	err := src.UnmarshalJSON(state)
	if err != nil {
		return fmt.Errorf("failed to unmarshal app state: %v", err)
	}

	dst := m.db.GetDB().DB
	err = dst.EndBatch(src.Export())
	if err != nil {
		return fmt.Errorf("failed to load app state into database: %v", err)
	}

	return nil
}

// BeginBlock implements ./abci.Chain
func (m *Executor) BeginBlock(req abci.BeginBlockRequest) (abci.BeginBlockResponse, error) {
	m.leader = req.IsLeader
	m.height = req.Height
	m.time = req.Time
	m.chainWG = make(map[uint64]*sync.WaitGroup, chainWGSize)
	m.dbTx = m.db.Begin()

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

	// If we're the leader, sign synthetic transactions produced by the previous
	// block
	if m.leader {
		err := m.signSynthTxns()
		if err != nil {
			return abci.BeginBlockResponse{}, err
		}
	}

	// Send synthetic transactions produced in the previous block
	txns, err := m.sendSynthTxns()
	if err != nil {
		return abci.BeginBlockResponse{}, err
	}

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
		m.dbTx = m.db.Begin()
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

	sigGroup := new(protocol.SigSpecGroup)
	switch sponsor := st.Sponsor.(type) {
	case *protocol.LiteTokenAccount:
		return st, m.checkAnonymous(st, tx, sponsor)

	case *state.AdiState, *state.TokenAccount, *protocol.SigSpec:
		if (sponsor.Header().SigSpecId == types.Bytes32{}) {
			return nil, fmt.Errorf("sponsor has not been assigned to an SSG")
		}
		err := st.LoadAs(sponsor.Header().SigSpecId, sigGroup)
		if err != nil {
			return nil, fmt.Errorf("invalid SigSpecId: %v", err)
		}

	case *protocol.SigSpecGroup:
		sigGroup = sponsor

	default:
		// The TX sponsor cannot be a transaction
		// Token issue chains are not implemented
		return nil, fmt.Errorf("invalid sponsor: chain type %v cannot sponsor transactions", sponsor.Header().Type)
	}

	if tx.SigInfo.PriorityIdx >= uint64(len(sigGroup.SigSpecs)) {
		return nil, fmt.Errorf("invalid sig spec index")
	}

	sigSpec := new(protocol.SigSpec)
	err = st.LoadAs(sigGroup.SigSpecs[tx.SigInfo.PriorityIdx], sigSpec)
	if err != nil {
		return nil, fmt.Errorf("invalid sig spec: %v", err)
	}

	// TODO check height

	for i, sig := range tx.Signature {
		ks := sigSpec.FindKey(sig.PublicKey)
		if ks == nil {
			return nil, fmt.Errorf("no key spec matches signature %d", i)
		}

		if ks.Nonce >= sig.Nonce {
			return nil, fmt.Errorf("invalid nonce")
		}
		// TODO add pending update for the nonce
	}

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

func (m *Executor) checkAnonymous(st *StateManager, tx *transactions.GenTransaction, account *protocol.LiteTokenAccount) error {
	u, err := account.ParseUrl()
	if err != nil {
		// This shouldn't happen because invalid URLs should never make it
		// into the database.
		return fmt.Errorf("invalid sponsor URL: %v", err)
	}

	urlKH, _, err := protocol.ParseAnonymousAddress(u)
	if err != nil {
		// This shouldn't happen because invalid URLs should never make it
		// into the database.
		return fmt.Errorf("invalid anonymous token URL: %v", err)
	}

	for i, sig := range tx.Signature {
		sigKH := sha256.Sum256(sig.PublicKey)
		if !bytes.Equal(urlKH, sigKH[:20]) {
			return fmt.Errorf("signature %d's public key does not match the sponsor", i)
		}

		if account.Nonce >= sig.Nonce {
			return fmt.Errorf("invalid nonce")
		}
	}

	// TODO add pending update for the nonce

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

	txt := types.TxType(tx.TransactionType())
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

	mdRoot, err := m.dbTx.Commit(m.height, m.time)
	if err != nil {
		return nil, err
	}

	m.logger.Info("Committed", "db_time", m.db.TimeBucket)
	m.db.TimeBucket = 0
	return mdRoot, nil
}

// synthCount returns the number of synthetic transactions sent by this subnet.
func (m *Executor) synthCount() (uint64, error) {
	k := storage.ComputeKey("SyntheticTransactionCount")
	b, err := m.dbTx.Read(k)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return 0, err
	}

	var n uint64
	if len(b) > 0 {
		n, _ = common.BytesUint64(b)
	}
	return n, nil
}

// nextSynthCount returns and increments the number of synthetic transactions
// sent by this subnet.
func (m *Executor) nextSynthCount() (uint64, error) {
	// TODO Replace this with the actual key nonce

	n, err := m.synthCount()
	if err != nil {
		return 0, err
	}

	k := storage.ComputeKey("SyntheticTransactionCount")
	m.dbTx.Write(k, common.Uint64Bytes(n+1))
	return n, nil
}

// addSynthTxns prepares synthetic transactions for signing next block.
func (m *Executor) addSynthTxns(parentTxId types.Bytes, st *StateManager) error {
	// Need to pass this to a threaded batcher / dispatcher to do both signing
	// and sending of synth tx. No need to spend valuable time here doing that.
	for _, sub := range st.submissions {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		// Marshal the payload
		body, err := sub.body.MarshalBinary()
		if err != nil {
			return fmt.Errorf("failed to marshal synthetic transaction payload: %v", err)
		}

		// Build the transaction
		tx := new(transactions.GenTransaction)
		tx.SigInfo = new(transactions.SignatureInfo)
		tx.SigInfo.URL = sub.url.String()
		tx.SigInfo.MSHeight = 1
		tx.SigInfo.PriorityIdx = 0
		tx.Transaction = body

		tx.SigInfo.MSHeight = 1
		tx.SigInfo.Nonce, err = m.nextSynthCount()
		if err != nil {
			return err
		}

		// Create the state object to store the unsigned pending transaction
		txSynthetic := state.NewPendingTransaction(tx)
		txSyntheticObject := new(state.Object)
		synthTxData, err := txSynthetic.MarshalBinary()
		if err != nil {
			return err
		}
		txSyntheticObject.Entry = synthTxData
		m.dbTx.AddSynthTx(parentTxId, tx.TransactionHash(), txSyntheticObject)
	}

	return nil
}

// signSynthTxns signs synthetic transactions from the previous block and
// prepares them to be sent next block.
func (m *Executor) signSynthTxns() error {
	// TODO If the leader fails, will the block happen again or will Tendermint
	// move to the next block? We need to be sure that synthetic transactions
	// won't get lost.

	// Get the anchor chain head
	head, height, err := m.db.GetAnchorHead()
	if errors.Is(err, storage.ErrNotFound) {
		// Nothing to do
		return nil
	} else if err != nil {
		return err
	}

	// Only proceed if the previous block did something
	if head.Index != m.height-1 {
		return nil
	}

	// Only proceed if the previous block generated synthetic transactions
	count := height - head.PreviousHeight - int64(len(head.Chains)) - 1
	if count == 0 {
		return nil
	}

	// Pull the transaction IDs from the anchor chain
	txns, err := m.db.GetAnchors(height-count-1, height-1)
	if err != nil {
		return err
	}

	// Use the synthetic transaction count to calculate what the nonces were
	nonce, err := m.synthCount()
	if err != nil {
		return err
	}

	// Sign all of the transactions
	body := new(protocol.SyntheticSignTransactions)
	for i, txid := range txns {
		// For each pending synthetic transaction
		var synthSig protocol.SyntheticSignature
		synthSig.Txid = txid

		// The nonce must be the final nonce minus (I + 1)
		synthSig.Nonce = nonce - 1 - uint64(i)

		// Sign it
		ed := new(transactions.ED25519Sig)
		ed.PublicKey = m.key[32:]
		err = ed.Sign(synthSig.Nonce, m.key, txid[:])
		if err != nil {
			return err
		}

		// Add it to the list
		synthSig.Signature = ed.Signature
		body.Transactions = append(body.Transactions, synthSig)
	}

	// Construct the signature transaction
	tx := new(transactions.GenTransaction)
	tx.SigInfo = new(transactions.SignatureInfo)
	tx.SigInfo.URL = protocol.ACME
	tx.SigInfo.PriorityIdx = 0
	tx.Transaction, err = body.MarshalBinary()
	if err != nil {
		return err
	}
	tx.SigInfo.MSHeight = 1
	tx.SigInfo.Nonce, err = m.nextSynthCount()
	if err != nil {
		return err
	}

	// Sign it
	ed := new(transactions.ED25519Sig)
	tx.Signature = append(tx.Signature, ed)
	ed.PublicKey = m.key[32:]
	err = ed.Sign(tx.SigInfo.Nonce, m.key, tx.TransactionHash())
	if err != nil {
		return err
	}

	// Marshal it
	data, err := tx.Marshal()
	if err != nil {
		return err
	}

	// Send it
	go m.local.BroadcastTxAsync(context.Background(), data)
	return nil
}

// sendSynthTxns sends signed synthetic transactions from previous blocks.
func (m *Executor) sendSynthTxns() ([]abci.SynthTxnReference, error) {
	// Get the signatures from the last block
	sigs, err := m.db.GetSynthTxnSigs()
	if err != nil {
		return nil, err
	}

	// Array for synth TXN references
	refs := make([]abci.SynthTxnReference, 0, len(sigs))

	// Process all the transactions
	for _, sig := range sigs {
		// Load the pending transaction object
		obj, err := m.db.GetSynthTxn(sig.Txid)
		if err != nil {
			return nil, err
		}

		// Unmarshal it
		state := new(state.PendingTransaction)
		err = obj.As(state)
		if err != nil {
			return nil, err
		}

		// Convert it back to a transaction
		tx := state.Restore()

		// Add the signature
		tx.Signature = append(tx.Signature, &transactions.ED25519Sig{
			Nonce:     sig.Nonce,
			PublicKey: sig.PublicKey,
			Signature: sig.Signature,
		})

		// Add it to the batch
		ti, err := m.query.BroadcastTx(tx, nil)
		if err != nil {
			continue
		}

		// Delete the signature
		m.dbTx.DeleteSynthTxnSig(sig.Txid)

		// Add the synthetic transaction reference
		var ref abci.SynthTxnReference
		ref.Type = uint64(tx.TransactionType())
		ref.Url = tx.SigInfo.URL
		copy(ref.Hash[:], tx.TransactionHash())
		copy(ref.TxRef[:], ti.ReferenceId)
		refs = append(refs, ref)
	}

	// Send the batches
	m.query.BatchSend()

	return refs, nil
}
