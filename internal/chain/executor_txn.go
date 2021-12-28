package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// CheckTx implements ./abci.Chain
func (m *Executor) CheckTx(tx *transactions.GenTransaction) *protocol.Error {
	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	executor, ok := m.executors[types.TxType(tx.TransactionType())]
	if !ok {
		return &protocol.Error{Code: protocol.CodeInvalidTxnType, Message: fmt.Errorf("unsupported TX type: %v", types.TxType(tx.TransactionType()))}
	}

	err := tx.SetRoutingChainID()
	if err != nil {
		return &protocol.Error{Code: protocol.CodeRoutingChainId, Message: err}
	}

	batch := m.DB.Begin()
	defer batch.Discard()

	st, err := m.check(batch, tx)
	if errors.Is(err, storage.ErrNotFound) {
		return &protocol.Error{Code: protocol.CodeNotFound, Message: err}
	}
	if err != nil {
		return &protocol.Error{Code: protocol.CodeCheckTxError, Message: err}
	}

	err = executor.Validate(st, tx)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeValidateTxnError, Message: err}
	}
	return nil
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
	chainId := types.Bytes(tx.ChainID).AsBytes32()
	if !ok {
		return m.recordTransactionError(tx, nil, nil, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeInvalidTxnType, Message: fmt.Errorf("unsupported TX type: %v", tx.TransactionType().Name())})
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

	st, err := m.check(m.blockBatch, tx)
	if err != nil {
		return m.recordTransactionError(tx, nil, nil, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeCheckTxError, Message: fmt.Errorf("txn check failed : %v", err)})
	}

	// Validate
	// TODO result should return a list of chainId's the transaction touched.
	err = executor.Validate(st, tx)
	if err != nil {
		return m.recordTransactionError(tx, nil, nil, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: fmt.Errorf("txn validation failed : %v", err)})
	}

	// If we get here, we were successful in validating.  So, we need to
	// split the transaction in 2, the body (i.e. TxAccepted), and the
	// validation material (i.e. TxPending).  The body of the transaction
	// gets put on the main chain, and the validation material gets put on
	// the pending chain which is purged after about 2 weeks
	txPending := state.NewPendingTransaction(tx)
	txAccepted, txPending := state.NewTransaction(txPending)
	txAcceptedObject := new(state.Object)
	txAcceptedObject.Entry, err = txAccepted.MarshalBinary()
	if err != nil {
		return m.recordTransactionError(tx, txAccepted, txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeMarshallingError, Message: err})
	}

	txPendingObject := new(state.Object)
	txPending.Status = json.RawMessage(fmt.Sprintf("{\"code\":\"0\"}"))
	txPendingObject.Entry, err = txPending.MarshalBinary()
	if err != nil {
		return m.recordTransactionError(tx, txAccepted, txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeMarshallingError, Message: err})
	}

	// Store the tx state
	status := &protocol.TransactionStatus{Delivered: true}
	err = m.putTransaction(tx, txAccepted, txPending, status)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeTxnStateError, Message: err}
	}

	// Store pending state updates, queue state creates for synthetic transactions
	meta, err := st.Commit()
	if err != nil {
		return m.recordTransactionError(tx, txAccepted, txPending, &chainId, tx.TransactionHash(), &protocol.Error{Code: protocol.CodeRecordTxnError, Message: err})
	}
	m.blockMeta.Deliver.Append(meta)

	// Process synthetic transactions generated by the validator
	err = m.addSynthTxns(tx.TransactionHash(), meta.Submitted)
	if err != nil {
		return &protocol.Error{Code: protocol.CodeSyntheticTxnError, Message: err}
	}

	m.blockMeta.Delivered++
	return nil
}

func (m *Executor) check(batch *database.Batch, tx *transactions.GenTransaction) (*StateManager, error) {
	if len(tx.Signature) == 0 {
		return nil, fmt.Errorf("transaction is not signed")
	}

	if !tx.ValidateSig() {
		return nil, fmt.Errorf("invalid signature")
	}

	txt := tx.TransactionType()

	st, err := NewStateManager(batch, m.Network.NodeUrl(), tx)
	if errors.Is(err, storage.ErrNotFound) {
		switch txt {
		case types.TxTypeSyntheticCreateChain, types.TxTypeSyntheticDepositTokens:
			// TX does not require an origin - it may create the origin
		default:
			return nil, fmt.Errorf("origin record not found: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	st.logger = m.logger

	if txt.IsSynthetic() {
		return st, m.checkSynthetic(st, tx)
	}

	book := new(protocol.KeyBook)
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		err = m.checkLite(st, tx, origin)
		if err != nil {
			return nil, err
		}
		return st, nil

	case *state.AdiState, *protocol.TokenAccount, *protocol.KeyPage, *protocol.DataAccount:
		if origin.Header().KeyBook == "" {
			return nil, fmt.Errorf("sponsor has not been assigned to a key book")
		}
		u, err := url.Parse(*origin.Header().KeyBook.AsString())
		if err != nil {
			return nil, fmt.Errorf("invalid keybook url %s", u.String())
		}
		err = st.LoadUrlAs(u, book)
		if err != nil {
			return nil, fmt.Errorf("invalid KeyBook: %v", err)
		}

	case *protocol.KeyBook:
		book = origin

	default:
		// The TX origin cannot be a transaction
		// Token issue chains are not implemented
		return nil, fmt.Errorf("invalid origin record: chain type %v cannot be the origininator of transactions", origin.Header().Type)
	}

	if tx.SigInfo.KeyPageIndex >= uint64(len(book.Pages)) {
		return nil, fmt.Errorf("invalid sig spec index")
	}

	u, err := url.Parse(book.Pages[tx.SigInfo.KeyPageIndex])
	if err != nil {
		return nil, fmt.Errorf("invalid key page url : %s", book.Pages[tx.SigInfo.KeyPageIndex])
	}
	page := new(protocol.KeyPage)
	err = st.LoadAs(u.ResourceChain32(), page)
	if err != nil {
		return nil, fmt.Errorf("invalid sig spec: %v", err)
	}

	height, err := st.GetHeight(u)
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
	err = st.UpdateNonce(page)
	if err != nil {
		return nil, err
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

func (m *Executor) checkLite(st *StateManager, tx *transactions.GenTransaction, account *protocol.LiteTokenAccount) error {
	u, err := account.ParseUrl()
	if err != nil {
		// This shouldn't happen because invalid URLs should never make it
		// into the database.
		return fmt.Errorf("invalid origin record URL: %v", err)
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
			return fmt.Errorf("signature %d's public key does not match the origin record", i)
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

	return st.UpdateNonce(account)
}

func (m *Executor) recordTransactionError(tx *transactions.GenTransaction, txAccepted *state.Transaction, txPending *state.PendingTransaction, chainId *types.Bytes32, txid []byte, failure *protocol.Error) *protocol.Error {
	status := &protocol.TransactionStatus{Delivered: true, Code: uint64(failure.Code)}
	err := m.putTransaction(tx, txAccepted, txPending, status)
	if err != nil {
		m.logError("Failed to store transaction", "txid", logging.AsHex(tx.TransactionHash()), "origin", tx.SigInfo.URL, "error", err)
	}
	return failure
}

func (m *Executor) putTransaction(tx *transactions.GenTransaction, txAccepted *state.Transaction, txPending *state.PendingTransaction, status *protocol.TransactionStatus) error {
	if txPending == nil {
		txPending = state.NewPendingTransaction(tx)
	}
	if txAccepted == nil {
		txAccepted, txPending = state.NewTransaction(txPending)
	}

	err := m.blockBatch.Transaction(tx.TransactionHash()).Put(txAccepted, status, tx.Signature)
	if err != nil {
		return fmt.Errorf("failed to store transaction: %v", err)
	}

	u, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return fmt.Errorf("invalid origin record URL: %v", err)
	}

	record := m.blockBatch.Record(u)
	chain, err := record.Chain(protocol.Pending)
	if err != nil {
		return fmt.Errorf("failed to load pending chain: %v", err)
	}

	for _, sig := range tx.Signature {
		txSig := new(protocol.TransactionSignature)
		copy(txSig.Transaction[:], tx.TransactionHash())
		txSig.Signature = sig

		data, err := txSig.MarshalBinary()
		if err != nil {
			return fmt.Errorf("failed to marshal signature: %v", err)
		}

		sigId := sha256.Sum256(data)
		record.Index("Signature", sigId).Put(data)
		err = chain.AddEntry(sigId[:])
		if err != nil {
			return fmt.Errorf("failed to add signature to pending chain: %v", err)
		}
	}

	return nil
}
