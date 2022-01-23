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
func (m *Executor) CheckTx(env *transactions.Envelope) (protocol.TransactionResult, *protocol.Error) {
	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	txt := types.TxType(env.Transaction.Type())
	executor, ok := m.executors[txt]
	if !ok {
		return nil, &protocol.Error{Code: protocol.CodeInvalidTxnType, Message: fmt.Errorf("unsupported TX type: %v", txt)}
	}

	batch := m.DB.Begin()
	defer batch.Discard()

	st, err := m.check(batch, env)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, &protocol.Error{Code: protocol.CodeNotFound, Message: err}
	}
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeCheckTxError, Message: err}
	}

	// Do not run transaction-specific validation for a synthetic transaction. A
	// synthetic transaction will be rejected by `m.check` unless it is signed
	// by a BVN and can be proved to have been included in a DN block. If
	// `m.check` succeeeds, we know the transaction came from a BVN, thus it is
	// safe and reasonable to allow the transaction to be delivered.
	//
	// This is important because if a synthetic transaction is rejected during
	// CheckTx, it does not get recorded. If the synthetic transaction is not
	// recorded, the BVN that sent it and the client that sent the original
	// transaction cannot verify that the synthetic transaction was received.
	if txt.IsSynthetic() {
		return new(protocol.EmptyResult), nil
	}

	result, err := executor.Validate(st, env)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeValidateTxnError, Message: err}
	}
	if result == nil {
		result = new(protocol.EmptyResult)
	}
	return result, nil
}

// DeliverTx implements ./abci.Chain
func (m *Executor) DeliverTx(env *transactions.Envelope) (protocol.TransactionResult, *protocol.Error) {
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

	if env.Transaction.Origin == nil {
		return nil, &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: fmt.Errorf("malformed transaction error")}
	}

	txt := env.Transaction.Type()
	executor, ok := m.executors[txt]
	if !ok {
		return nil, m.recordTransactionError(nil, env, nil, nil, false, &protocol.Error{
			Code:    protocol.CodeInvalidTxnType,
			Message: fmt.Errorf("unsupported TX type: %v", env.Transaction.Type().Name())})
	}

	// if txt.IsInternal() && tx.Transaction.Nonce != uint64(m.blockIndex) {
	// 	err := fmt.Errorf("nonce does not match block index, want %d, got %d", m.blockIndex, tx.Transaction.Nonce)
	// 	return nil, m.recordTransactionError(tx, nil, nil, &chainId, tx.Transaction.Hash(), &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: err})
	// }

	m.mu.Lock()
	group, ok := m.chainWG[env.Transaction.Origin.Routing()%chainWGSize]
	if !ok {
		group = new(sync.WaitGroup)
		m.chainWG[env.Transaction.Origin.Routing()%chainWGSize] = group
	}

	group.Wait()
	group.Add(1)
	defer group.Done()
	m.mu.Unlock()

	st, err := m.check(m.blockBatch, env)
	if err != nil {
		return nil, m.recordTransactionError(nil, env, nil, nil, false, &protocol.Error{Code: protocol.CodeCheckTxError, Message: fmt.Errorf("txn check failed : %v", err)})
	}

	result, err := executor.Validate(st, env)
	if err != nil {
		return nil, m.recordTransactionError(st, env, nil, nil, false, &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: fmt.Errorf("txn validation failed : %v", err)})
	}
	if result == nil {
		result = new(protocol.EmptyResult)
	}

	// If we get here, we were successful in validating.  So, we need to
	// split the transaction in 2, the body (i.e. TxAccepted), and the
	// validation material (i.e. TxPending).  The body of the transaction
	// gets put on the main chain, and the validation material gets put on
	// the pending chain which is purged after about 2 weeks
	txPending := state.NewPendingTransaction(env)
	txAccepted, txPending := state.NewTransaction(txPending)
	txAcceptedObject := new(state.Object)
	txAcceptedObject.Entry, err = txAccepted.MarshalBinary()
	if err != nil {
		return nil, m.recordTransactionError(st, env, txAccepted, txPending, false, &protocol.Error{Code: protocol.CodeMarshallingError, Message: err})
	}

	txPendingObject := new(state.Object)
	txPending.Status = json.RawMessage(fmt.Sprintf("{\"code\":\"0\"}"))
	txPendingObject.Entry, err = txPending.MarshalBinary()
	if err != nil {
		return nil, m.recordTransactionError(st, env, txAccepted, txPending, false, &protocol.Error{Code: protocol.CodeMarshallingError, Message: err})
	}

	// Store pending state updates, queue state creates for synthetic transactions
	submitted, err := st.Commit()
	if err != nil {
		return nil, m.recordTransactionError(st, env, txAccepted, txPending, true, &protocol.Error{Code: protocol.CodeRecordTxnError, Message: err})
	}

	// Store the tx state
	status := &protocol.TransactionStatus{Delivered: true, Result: result}
	err = m.putTransaction(st, env, txAccepted, txPending, status, false)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeTxnStateError, Message: err}
	}

	// Process synthetic transactions generated by the validator
	st.Reset()
	err = m.addSynthTxns(&st.stateCache, submitted)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeSyntheticTxnError, Message: err}
	}
	_, err = st.Commit()
	if err != nil {
		return nil, m.recordTransactionError(st, env, txAccepted, txPending, true, &protocol.Error{Code: protocol.CodeRecordTxnError, Message: err})
	}

	m.blockMeta.Delivered++
	return result, nil
}

func (m *Executor) check(batch *database.Batch, env *transactions.Envelope) (*StateManager, error) {
	if len(env.Signatures) == 0 {
		return nil, fmt.Errorf("transaction is not signed")
	}

	if !env.Verify() {
		return nil, fmt.Errorf("invalid signature")
	}

	txt := env.Transaction.Type()

	st, err := NewStateManager(batch, m.Network.NodeUrl(), env)
	if errors.Is(err, storage.ErrNotFound) {
		switch txt {
		case types.TxTypeSyntheticCreateChain, types.TxTypeSyntheticDepositTokens, types.TxTypeSyntheticWriteData:
			// TX does not require the origin record to exist
		default:
			return nil, fmt.Errorf("origin record not found: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	st.logger.L = m.logger

	if txt.IsSynthetic() {
		return st, m.checkSynthetic(st, env)
	}

	if txt.IsInternal() {
		return st, m.checkInternal(st, env)
	}

	book := new(protocol.KeyBook)
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		err = m.checkLite(st, env)
		if err != nil {
			return nil, err
		}
		return st, nil

	case *protocol.ADI, *protocol.TokenAccount, *protocol.KeyPage, *protocol.DataAccount, *protocol.TokenIssuer:
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
		return nil, fmt.Errorf("invalid origin record: account type %v cannot be the origininator of transactions", origin.Header().Type)
	}

	if env.Transaction.KeyPageIndex >= uint64(len(book.Pages)) {
		return nil, fmt.Errorf("invalid sig spec index")
	}

	st.SignatorUrl, err = url.Parse(book.Pages[env.Transaction.KeyPageIndex])
	if err != nil {
		return nil, fmt.Errorf("invalid key page url : %s", book.Pages[env.Transaction.KeyPageIndex])
	}
	page := new(protocol.KeyPage)
	st.Signator = page
	err = st.LoadUrlAs(st.SignatorUrl, page)
	if err != nil {
		return nil, fmt.Errorf("invalid sig spec: %v", err)
	}

	height, err := st.GetHeight(st.SignatorUrl)
	if err != nil {
		return nil, err
	}
	if height != env.Transaction.KeyPageHeight {
		return nil, fmt.Errorf("invalid height")
	}

	for i, sig := range env.Signatures {
		ks := page.FindKey(sig.PublicKey)
		if ks == nil {
			return nil, fmt.Errorf("no key spec matches signature %d", i)
		}

		switch {
		case i > 0:
			// Only check the nonce of the first key
		case ks.Nonce >= sig.Nonce:
			return nil, fmt.Errorf("invalid nonce: have %d, received %d", ks.Nonce, sig.Nonce)
		default:
			ks.Nonce = sig.Nonce
		}
	}

	cost, err := protocol.ComputeFee(env)
	if err != nil {
		return nil, err
	}
	if !page.DebitCredits(uint64(cost)) {
		return nil, fmt.Errorf("insufficent credits for the transaction: %q has %v, cost is %d", page.ChainUrl, page.CreditBalance.String(), cost)
	}

	err = st.UpdateSignator(page)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (m *Executor) checkSynthetic(st *StateManager, env *transactions.Envelope) error {
	//placeholder for special validation rules for synthetic transactions.
	//need to verify the sender is a legit bvc validator also need the dbvc receipt
	//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
	//not an impostor. Need to figure out how to do this. Right now we just assume the synth request
	//sender is legit.

	// TODO Get rid of this hack and actually check the nonce. But first we have
	// to implement transaction batching.
	v := st.RecordIndex(m.Network.NodeUrl(), "SeenSynth", env.Transaction.Hash())
	_, err := v.Get()
	if err == nil {
		return fmt.Errorf("duplicate synthetic transaction %X", env.Transaction.Hash())
	} else if errors.Is(err, storage.ErrNotFound) {
		v.Put([]byte{1})
	} else {
		return err
	}

	return nil
}

func (m *Executor) checkInternal(st *StateManager, env *transactions.Envelope) error {
	//placeholder for special validation rules for internal transactions.
	//need to verify that the transaction came from one of the node's governors.
	return nil
}

func (m *Executor) checkLite(st *StateManager, env *transactions.Envelope) error {
	account := st.Origin.(*protocol.LiteTokenAccount)
	st.Signator = account
	st.SignatorUrl = st.OriginUrl

	urlKH, _, err := protocol.ParseLiteTokenAddress(st.OriginUrl)
	if err != nil {
		// This shouldn't happen because invalid URLs should never make it
		// into the database.
		return fmt.Errorf("invalid lite token URL: %v", err)
	}

	for i, sig := range env.Signatures {
		sigKH := sha256.Sum256(sig.PublicKey)
		if !bytes.Equal(urlKH, sigKH[:20]) {
			return fmt.Errorf("signature %d's public key does not match the origin record", i)
		}

		switch {
		case i > 0:
			// Only check the nonce of the first key
		case account.Nonce >= sig.Nonce:
			return fmt.Errorf("invalid nonce: have %d, received %d", account.Nonce, sig.Nonce)
		default:
			account.Nonce = sig.Nonce
		}
	}

	cost, err := protocol.ComputeFee(env)
	if err != nil {
		return err
	}
	if !account.DebitCredits(uint64(cost)) {
		return fmt.Errorf("insufficent credits for the transaction: %q has %v, cost is %d", account.ChainUrl, account.CreditBalance.String(), cost)
	}

	return st.UpdateSignator(account)
}

func (m *Executor) recordTransactionError(st *StateManager, env *transactions.Envelope, txAccepted *state.Transaction, txPending *state.PendingTransaction, postCommit bool, failure *protocol.Error) *protocol.Error {
	status := &protocol.TransactionStatus{
		Delivered: true,
		Code:      uint64(failure.Code),
		Message:   failure.Error(),
	}
	err := m.putTransaction(st, env, txAccepted, txPending, status, postCommit)
	if err != nil {
		m.logError("Failed to store transaction", "txid", logging.AsHex(env.Transaction.Hash()), "origin", env.Transaction.Origin, "error", err)
	}
	return failure
}

func (m *Executor) putTransaction(st *StateManager, env *transactions.Envelope, txAccepted *state.Transaction, txPending *state.PendingTransaction, status *protocol.TransactionStatus, postCommit bool) error {
	if txPending == nil {
		txPending = state.NewPendingTransaction(env)
	}
	if txAccepted == nil {
		txAccepted, txPending = state.NewTransaction(txPending)
	}

	// Don't add internal transactions to chains. Internal transactions are
	// exclusively used for communication between the governor and the state
	// machine.
	txt := env.Transaction.Type()
	if txt.IsInternal() {
		return nil
	}

	// Store against the transaction hash
	err := m.blockBatch.Transaction(env.GetTxHash()).Put(txAccepted, status, env.Signatures)
	if err != nil {
		return fmt.Errorf("failed to store transaction: %v", err)
	}

	// Store against the envelope hash
	err = m.blockBatch.Transaction(env.EnvHash()).Put(txAccepted, status, env.Signatures)
	if err != nil {
		return fmt.Errorf("failed to store transaction: %v", err)
	}

	if st == nil {
		return nil // Check failed
	}

	if postCommit {
		return nil // Failure after commit
	}

	// Add the envelope to the origin's pending chain
	err = addChainEntry(m.Network.NodeUrl(), m.blockBatch, st.OriginUrl, protocol.PendingChain, protocol.ChainTypeTransaction, env.EnvHash(), 0, 0)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("failed to add signature to pending chain: %v", err)
	}

	// The signator of a synthetic transaction is the subnet ADI (acc://dn or
	// acc://bvn-{id}). Do not add transactions to the subnet's pending chain
	// and do not charge fees to the subnet's ADI.
	if txt.IsSynthetic() {
		return nil
	}

	// If the origin and signator are different, add the envelope to the signator's pending chain
	if !st.OriginUrl.Equal(st.SignatorUrl) {
		err = addChainEntry(m.Network.NodeUrl(), m.blockBatch, st.SignatorUrl, protocol.PendingChain, protocol.ChainTypeTransaction, env.EnvHash(), 0, 0)
		if err != nil {
			return fmt.Errorf("failed to add signature to pending chain: %v", err)
		}
	}

	if status.Code == protocol.CodeOK {
		return nil
	}

	// Update the nonce and charge the failure transaction fee. Reload the
	// signator to ensure we don't push any invalid changes. Use the database
	// directly, since the state manager won't be committed.
	sigRecord := m.blockBatch.Account(st.SignatorUrl)
	err = sigRecord.GetStateAs(st.Signator)
	if err != nil {
		return err
	}

	sig := env.Signatures[0]
	err = st.Signator.SetNonce(sig.PublicKey, sig.Nonce)
	if err != nil {
		return err
	}

	fee, err := protocol.ComputeFee(env)
	if err != nil || fee > protocol.FeeFailedMaximum.AsInt() {
		fee = protocol.FeeFailedMaximum.AsInt()
	}
	st.Signator.DebitCredits(uint64(fee))

	return sigRecord.PutState(st.Signator)
}
