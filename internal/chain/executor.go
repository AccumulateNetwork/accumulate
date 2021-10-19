package chain

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/accumulated/internal/abci"
	accapi "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

const chainWGSize = 4

type Executor struct {
	db        *state.StateDB
	key       ed25519.PrivateKey
	query     *accapi.Query
	executors map[types.TxType]TxExecutor

	wg      *sync.WaitGroup
	mu      *sync.Mutex
	chainWG map[uint64]*sync.WaitGroup
	leader  bool
	height  int64
	nonce   uint64 //global nonce for synth tx's, needs to be managed in bvc/symnthsigstate.
}

var _ abci.Chain = (*Executor)(nil)

func NewExecutor(query *accapi.Query, db *state.StateDB, key ed25519.PrivateKey, executors ...TxExecutor) (*Executor, error) {
	m := new(Executor)
	m.db = db
	m.executors = map[types.TxType]TxExecutor{}
	m.key = key
	m.wg = new(sync.WaitGroup)
	m.mu = new(sync.Mutex)
	m.query = query

	for _, x := range executors {
		if _, ok := m.executors[x.Type()]; ok {
			panic(fmt.Errorf("duplicate executor for %d", x.Type()))
		}
		m.executors[x.Type()] = x
	}

	fmt.Printf("Loaded height=%d hash=%X\n", db.BlockIndex(), db.EnsureRootHash())
	return m, nil
}

func (m *Executor) Query(q *api.Query) ([]byte, error) {
	if q.Content != nil {
		tx, pendingTx, synthTxIds, err := m.db.GetTx(q.Content)
		if err != nil {
			return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
		}
		ret := append(common.SliceBytes(tx), common.SliceBytes(pendingTx)...)
		ret = append(ret, common.SliceBytes(synthTxIds)...)
		return ret, nil
	}

	chainState, err := m.db.GetCurrentEntry(q.ChainId)
	if err != nil {
		return nil, fmt.Errorf("failed to locate chain entry: %v", err)
	}

	err = chainState.As(new(state.ChainHeader))
	if err != nil {
		return nil, fmt.Errorf("unable to extract chain header: %v", err)
	}

	return chainState.Entry, nil
}

// BeginBlock implements ./abci.Chain
func (m *Executor) BeginBlock(req abci.BeginBlockRequest) {
	m.leader = req.IsLeader
	m.height = req.Height
	m.chainWG = make(map[uint64]*sync.WaitGroup, chainWGSize)
}

func (m *Executor) check(st *state.StateEntry, tx *transactions.GenTransaction) error {
	if len(tx.Signature) == 0 {
		return fmt.Errorf("transaction is not signed")
	}

	txt := tx.TransactionType()
	if txt.IsSynthetic() {
		return m.checkSynthetic(st, tx)
	}

	if st.ChainHeader == nil {
		return fmt.Errorf("sponsor not found")
	}

	if !tx.ValidateSig() {
		return fmt.Errorf("invalid signature")
	}

	ssg := new(protocol.SigSpecGroup)
	switch st.ChainHeader.Type {
	case types.ChainTypeAnonTokenAccount:
		return m.checkAnonymous(st, tx)

	case types.ChainTypeAdi, types.ChainTypeTokenAccount, types.ChainTypeSigSpec:
		_, err := m.db.LoadChainAs(st.ChainHeader.SigSpecId[:], ssg)
		if err != nil {
			return fmt.Errorf("failed to load sig spec group: %v", err)
		}

	case types.ChainTypeSigSpecGroup:
		err := st.ChainState.As(ssg)
		if err != nil {
			return fmt.Errorf("failed to decode sponsor: %v", err)
		}

	default:
		// The TX sponsor cannot be a transaction
		// Token issue chains are not implemented
		return fmt.Errorf("%v cannot sponsor transactions", st.ChainHeader.Type)
	}

	if tx.SigInfo.PriorityIdx >= uint64(len(ssg.SigSpecs)) {
		return fmt.Errorf("invalid sig spec index")
	}

	ss := new(protocol.SigSpec)
	_, err := m.db.LoadChainAs(ssg.SigSpecs[tx.SigInfo.PriorityIdx][:], ss)
	if err != nil {
		return fmt.Errorf("failed to load sig spec: %v", err)
	}

	// TODO check height

	for i, sig := range tx.Signature {
		ks, err := ss.FindKey(sig.PublicKey, protocol.ED25519)
		if err != nil {
			return fmt.Errorf("failed to verify signature %d: %v", i, err)
		}

		if ks == nil {
			return fmt.Errorf("no key spec matches signature %d", i)
		}

		if ks.Nonce >= sig.Nonce {
			return fmt.Errorf("invalid nonce")
		}
		// TODO add pending update for the nonce
	}

	return nil
}

func (m *Executor) checkSynthetic(st *state.StateEntry, tx *transactions.GenTransaction) error {
	//placeholder for special validation rules for synthetic transactions.
	//need to verify the sender is a legit bvc validator also need the dbvc receipt
	//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
	//not an impostor. Need to figure out how to do this. Right now we just assume the synth request
	//sender is legit.

	if !tx.ValidateSig() {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func (m *Executor) checkAnonymous(st *state.StateEntry, tx *transactions.GenTransaction) error {
	account := new(protocol.AnonTokenAccount)
	err := st.ChainState.As(account)
	if err != nil {
		return fmt.Errorf("failed to decode sponsor: %v", err)
	}

	u, err := st.ChainHeader.ParseUrl()
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
func (m *Executor) CheckTx(tx *transactions.GenTransaction) error {
	err := tx.SetRoutingChainID()
	if err != nil {
		return err
	}

	m.mu.Lock()
	st, err := m.db.LoadChainState(tx.ChainID)
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to get state for : %v", err)
	}

	err = m.check(st, tx)
	if err != nil {
		return err
	}

	executor, ok := m.executors[types.TxType(tx.TransactionType())]
	if !ok {
		return fmt.Errorf("unsupported TX type: %v", types.TxType(tx.TransactionType()))
	}

	return executor.CheckTx(st, tx)
}

// DeliverTx implements ./abci.Chain
func (m *Executor) DeliverTx(tx *transactions.GenTransaction) (*protocol.TxResult, error) {
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
		return nil, fmt.Errorf("malformed transaction")
	}

	executor, ok := m.executors[types.TxType(tx.TransactionType())]
	if !ok {
		return nil, fmt.Errorf("unsupported TX type: %v", types.TxType(tx.TransactionType()))
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

	st, err := m.db.LoadChainState(tx.ChainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get state: %v", err)
	}

	err = m.check(st, tx)
	if err != nil {
		return nil, err
	}

	// First configure the pending state which is the basis for the transaction
	txPending := state.NewPendingTransaction(tx)

	// Validate
	// TODO result should return a list of chainId's the transaction touched.
	result, err := executor.DeliverTx(st, tx)
	if err != nil {
		return nil, fmt.Errorf("rejected by chain: %v", err)
	}

	// Check if the transaction was accepted
	var txAcceptedObject *state.Object

	if err == nil {
		// If we get here, we were successful in validating.  So, we need to
		// split the transaction in 2, the body (i.e. TxAccepted), and the
		// validation material (i.e. TxPending).  The body of the transaction
		// gets put on the main chain, and the validation material gets put on
		// the pending chain which is purged after about 2 weeks
		var txAccepted *state.Transaction
		txAccepted, txPending = state.NewTransaction(txPending)
		txAcceptedObject = new(state.Object)
		txAcceptedObject.Entry, err = txAccepted.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}

	txPendingObject := new(state.Object)
	txPendingObject.Entry, err = txPending.MarshalBinary()
	if err != nil {
		return nil, err
	}

	// Store pending state changes
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	for id, chain := range result.Chains {
		data, err := chain.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal state: %v", err)
		}
		m.db.AddStateEntry((*types.Bytes32)(&id), &txHash, &state.Object{Entry: data})
	}

	// Store the tx state
	var chainId types.Bytes32
	copy(chainId[:], tx.ChainID)
	err = m.db.AddPendingTx(&chainId, tx.TransactionHash(), txPendingObject, txAcceptedObject)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, errors.New("no chain validation")
	}

	// Process synthetic transactions generated by the validator
	refs, err := m.submitSyntheticTx(tx.TransactionHash(), result)
	if err != nil {
		return nil, err
	}

	r := new(protocol.TxResult)
	r.SyntheticTxs = refs
	return r, nil
}

// EndBlock implements ./abci.Chain
func (m *Executor) EndBlock(req abci.EndBlockRequest) {}

// Commit implements ./abci.Chain
func (m *Executor) Commit() ([]byte, error) {
	m.wg.Wait()

	mdRoot, numStateChanges, err := m.db.WriteStates(m.height)
	if err != nil {
		// This should never happen
		panic(fmt.Errorf("fatal error, block not set, %v", err))
	}

	// If we have no transactions this block then don't publish anything
	if m.leader && numStateChanges > 0 {
		// Now we create a synthetic transaction and publish to the directory
		// block validator
		dbvc := DeliverTxResult{}
		dbvc.SyntheticTransactions = make([]*transactions.GenTransaction, 1)
		dbvc.SyntheticTransactions[0] = &transactions.GenTransaction{}
		dcAdi := "dc"
		dbvc.SyntheticTransactions[0].ChainID = types.GetChainIdFromChainPath(&dcAdi).Bytes()
		dbvc.SyntheticTransactions[0].Routing = types.GetAddressFromIdentity(&dcAdi)
		//dbvc.Submissions[0].Transaction = ...

		//broadcast the root
		//m.processValidatedSubmissionRequest(&dbvc)
	}

	m.query.BatchSend()

	fmt.Printf("DB time %f\n", m.db.TimeBucket)
	m.db.TimeBucket = 0
	return mdRoot, nil
}

func (m *Executor) submitSyntheticTx(parentTxId types.Bytes, vtx *DeliverTxResult) (tmRef []*protocol.TxSynthRef, err error) {
	if m.leader {
		tmRef = make([]*protocol.TxSynthRef, len(vtx.SyntheticTransactions))
	}

	// Need to pass this to a threaded batcher / dispatcher to do both signing
	// and sending of synth tx. No need to spend valuable time here doing that.
	for i, tx := range vtx.SyntheticTransactions {
		// Generate a synthetic tx and send to the router. Need to track txid to
		// make sure they get processed.

		if tx.Transaction == nil {
			// This should never happen
			return nil, fmt.Errorf("submission is missing its synthetic transaction")
		}

		if tx.SigInfo == nil {
			// This should never happen
			return nil, fmt.Errorf("synthetic transaction is missing its signature info")
		}

		tx.SigInfo.Unused2 = m.nonce
		m.nonce++ //TODO: make the nonce managed via the BVC admin state for synth tx rather than this way

		// Create the state object to store the unsigned pending transaction
		txSynthetic := state.NewPendingTransaction(tx)
		txSyntheticObject := new(state.Object)
		synthTxData, err := txSynthetic.MarshalBinary()
		if err != nil {
			return nil, err
		}
		txSyntheticObject.Entry = synthTxData
		m.db.AddSynthTx(parentTxId, tx.TransactionHash(), txSyntheticObject)

		// TODO In order for other BVCs to be able to validate the syntetic
		// transaction, the signed version must be saved into the SMT. However,
		// this causes consensus to fail because the leader's state diverges
		// from non-leader nodes. So for now, the following block has been moved
		// to after the synthetic transaction is saved.

		// Batch synthetic transactions generated by the validator
		if m.leader {
			ed := new(transactions.ED25519Sig)
			//only if a leader we will need to sign and batch the tx's.
			//in future releases this will be submitted to this BVC to the next block for validation
			//of the synthetic tx by all the bvc nodes before being dispatched, along with DC receipt
			ed.PublicKey = m.key[32:]
			err := ed.Sign(tx.SigInfo.Unused2, m.key, tx.TransactionHash())
			if err != nil {
				return nil, fmt.Errorf("error signing sythetic transaction, %v", err)
			}

			tx.Signature = append(tx.Signature, ed)
			ti, err := m.query.BroadcastTx(tx, nil)
			if err != nil {
				return nil, err
			}

			tmRef[i] = new(protocol.TxSynthRef)
			tmRef[i].Type = uint64(tx.TransactionType())
			tmRef[i].Url = tx.SigInfo.URL
			copy(tmRef[i].Hash[:], tx.TransactionHash())
			copy(tmRef[i].TxRef[:], ti.ReferenceId)
		}
	}

	return tmRef, nil
}
