package chain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

// CheckTx implements ./abci.Chain
func (m *Executor) CheckTx(env *transactions.Envelope) (protocol.TransactionResult, *protocol.Error) {
	batch := m.DB.Begin()
	defer batch.Discard()

	st, executor, hasEnoughSigs, err := m.validate(batch, env)
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
	if env.Transaction.Type().IsSynthetic() {
		return new(protocol.EmptyResult), nil
	}

	// Do not validate if we don't have enough signatures
	if !hasEnoughSigs {
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
	// if txt.IsInternal() && tx.Transaction.Nonce != uint64(m.blockIndex) {
	// 	err := fmt.Errorf("nonce does not match block index, want %d, got %d", m.blockIndex, tx.Transaction.Nonce)
	// 	return nil, m.recordTransactionError(tx, nil, nil, &chainId, tx.GetTxHash(), &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: err})
	// }

	// Set up the state manager and validate the signatures
	st, executor, hasEnoughSigs, err := m.validate(m.blockBatch, env)
	if err != nil {
		return nil, m.recordTransactionError(nil, env, nil, nil, false, &protocol.Error{Code: protocol.CodeCheckTxError, Message: fmt.Errorf("txn check failed : %v", err)})
	}

	if !hasEnoughSigs {
		// Write out changes to the nonce and credit balance
		_, err := st.Commit()
		if err != nil {
			return nil, m.recordTransactionError(st, env, nil, nil, true, &protocol.Error{Code: protocol.CodeRecordTxnError, Message: err})
		}

		status := &protocol.TransactionStatus{Pending: true}
		err = m.putTransaction(st, env, nil, nil, status, false)
		if err != nil {
			return nil, &protocol.Error{Code: protocol.CodeTxnStateError, Message: err}
		}
		return new(protocol.EmptyResult), nil
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

	m.newValidators = append(m.newValidators, st.newValidators...)

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

// validate validates signatures, verifies they are authorized,
// updates the nonce, and charges the fee.
func (m *Executor) validate(batch *database.Batch, env *transactions.Envelope) (st *StateManager, executor TxExecutor, hasEnoughSigs bool, err error) {
	// Basic validation
	err = m.validateBasic(batch, env)
	if err != nil {
		return nil, nil, false, err
	}

	// Calculate the fee before modifying the transaction
	fee, err := protocol.ComputeFee(env)
	if err != nil {
		return nil, nil, false, err
	}

	// Load previous transaction state
	txt := env.Transaction.Type()
	txState, err := batch.Transaction(env.GetTxHash()).GetState()
	switch {
	case err == nil:
		// Populate the transaction from the database
		env.Transaction = txState.Restore().Transaction
		txt = env.Transaction.Type()

	case !errors.Is(err, storage.ErrNotFound):
		return nil, nil, false, fmt.Errorf("an error occured while looking up the transaction: %v", err)
	}

	// If this fails, the code is wrong
	executor, ok := m.executors[txt]
	if !ok {
		return nil, nil, false, fmt.Errorf("internal error: unsupported TX type: %v", txt)
	}

	// Set up the state manager
	st, err = NewStateManager(batch, m.Network.NodeUrl(), env)
	if errors.Is(err, storage.ErrNotFound) {
		switch txt {
		case types.TxTypeSyntheticCreateChain, types.TxTypeSyntheticDepositTokens, types.TxTypeSyntheticWriteData:
			// TX does not require the origin record to exist
		default:
			return nil, nil, false, fmt.Errorf("origin record not found: %w", err)
		}
	} else if err != nil {
		return nil, nil, false, err
	}
	st.logger.L = m.logger

	// Validate the transaction
	if txt.IsSynthetic() {
		return st, executor, true, m.validateSynthetic(st, env)
	}

	if txt.IsInternal() {
		return st, executor, true, m.validateInternal(st, env)
	}

	book := new(protocol.KeyBook)
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		return st, executor, true, m.validateAgainstLite(st, env, fee)

	case *protocol.ADI, *protocol.TokenAccount, *protocol.KeyPage, *protocol.DataAccount, *protocol.TokenIssuer:
		if origin.Header().KeyBook == "" {
			return nil, nil, false, fmt.Errorf("sponsor has not been assigned to a key book")
		}
		u, err := url.Parse(origin.Header().KeyBook)
		if err != nil {
			return nil, nil, false, fmt.Errorf("invalid keybook url %s", u.String())
		}
		err = st.LoadUrlAs(u, book)
		if err != nil {
			return nil, nil, false, fmt.Errorf("invalid KeyBook: %v", err)
		}

	case *protocol.KeyBook:
		book = origin

	default:
		// The TX origin cannot be a transaction
		// Token issue chains are not implemented
		return nil, nil, false, fmt.Errorf("invalid origin record: account type %v cannot be the origininator of transactions", origin.Header().Type)
	}

	hasEnoughSigs, err = m.validateAgainstBook(st, env, book, fee)
	return st, executor, hasEnoughSigs, err
}

func (m *Executor) validateBasic(batch *database.Batch, env *transactions.Envelope) error {
	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	txt := env.Transaction.Type()
	_, ok := m.executors[txt]
	if !ok && txt != types.TxTypeSignPending {
		return fmt.Errorf("unsupported TX type: %v", txt)
	}

	// All transactions must be signed
	if len(env.Signatures) == 0 {
		return fmt.Errorf("transaction is not signed")
	}

	switch {
	case len(env.TxHash) == 0:
		// TxHash must be specified for signature transactions
		if txt == types.TxTypeSignPending {
			return fmt.Errorf("invalid signature transaction: missing transaction hash")
		}

	case len(env.TxHash) != sha256.Size:
		// TxHash must either be empty or a SHA-256 hash
		return fmt.Errorf("invalid hash: not a SHA-256 hash")

	case !env.VerifyTxHash():
		// TxHash must match the transaction
		if txt != types.TxTypeSignPending {
			return fmt.Errorf("invalid hash: does not match transaction")
		}
	}

	// TxHash must either be empty or a SHA-256 hash
	if len(env.TxHash) != 0 && len(env.TxHash) != sha256.Size {
		return fmt.Errorf("invalid hash")
	}

	// TxHash must be specified for signature transactions
	if txt == types.TxTypeSignPending && len(env.TxHash) == 0 {
		return fmt.Errorf("invalid signature transaction: missing transaction hash")
	}

	// Verify the transaction's signatures
	if !env.Verify() {
		return fmt.Errorf("invalid signature(s)")
	}

	// Check the envelope
	_, err := batch.Transaction(env.EnvHash()).GetState()
	switch {
	case err == nil:
		return fmt.Errorf("duplicate envelope")
	case errors.Is(err, storage.ErrNotFound):
		// OK
	default:
		return fmt.Errorf("error while checking envelope state: %v", err)
	}

	// Check the transaction
	status, err := batch.Transaction(env.GetTxHash()).GetStatus()
	switch {
	case err == nil:
		if status.Delivered {
			return fmt.Errorf("transaction has already been delivered")
		}
	case errors.Is(err, storage.ErrNotFound):
		if txt == types.TxTypeSignPending {
			return fmt.Errorf("transaction not found")
		}
	default:
		return fmt.Errorf("failed to load transaction status: %v", err)
	}

	return nil
}

func (m *Executor) validateSynthetic(st *StateManager, env *transactions.Envelope) error {
	//placeholder for special validation rules for synthetic transactions.
	//need to verify the sender is a legit bvc validator also need the dbvc receipt
	//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
	//not an impostor. Need to figure out how to do this. Right now we just assume the synth request
	//sender is legit.

	// TODO Get rid of this hack and actually check the nonce. But first we have
	// to implement transaction batching.
	v := st.RecordIndex(m.Network.NodeUrl(), "SeenSynth", env.GetTxHash())
	_, err := v.Get()
	if err == nil {
		return fmt.Errorf("duplicate synthetic transaction %X", env.GetTxHash())
	} else if errors.Is(err, storage.ErrNotFound) {
		v.Put([]byte{1})
	} else {
		return err
	}

	return nil
}

func (m *Executor) validateInternal(st *StateManager, env *transactions.Envelope) error {
	//placeholder for special validation rules for internal transactions.
	//need to verify that the transaction came from one of the node's governors.
	return nil
}

func (m *Executor) validateAgainstBook(st *StateManager, env *transactions.Envelope, book *protocol.KeyBook, fee protocol.Fee) (bool, error) {
	if env.Transaction.KeyPageIndex >= uint64(len(book.Pages)) {
		return false, fmt.Errorf("invalid sig spec index")
	}

	var err error
	st.SignatorUrl, err = url.Parse(book.Pages[env.Transaction.KeyPageIndex])
	if err != nil {
		return false, fmt.Errorf("invalid key page url : %s", book.Pages[env.Transaction.KeyPageIndex])
	}
	page := new(protocol.KeyPage)
	st.Signator = page
	err = st.LoadUrlAs(st.SignatorUrl, page)
	if err != nil {
		return false, fmt.Errorf("invalid sig spec: %v", err)
	}

	height, err := st.GetHeight(st.SignatorUrl)
	if err != nil {
		return false, err
	}
	if height != env.Transaction.KeyPageHeight {
		return false, fmt.Errorf("invalid height: want %d, got %d", height, env.Transaction.KeyPageHeight)
	}

	for i, sig := range env.Signatures {
		ks := page.FindKey(sig.PublicKey)
		if ks == nil {
			return false, fmt.Errorf("no key spec matches signature %d", i)
		}

		switch {
		case i > 0:
			// Only check the nonce of the first key
		case ks.Nonce >= sig.Nonce:
			return false, fmt.Errorf("invalid nonce: have %d, received %d", ks.Nonce, sig.Nonce)
		default:
			ks.Nonce = sig.Nonce
		}
	}

	if !page.DebitCredits(uint64(fee)) {
		return false, fmt.Errorf("insufficent credits for the transaction: %q has %v, cost is %d", page.Url, page.CreditBalance.String(), fee)
	}

	err = st.UpdateSignator(page)
	if err != nil {
		return false, err
	}

	sigCount, err := st.batch.Transaction(env.GetTxHash()).AddSignatures(env.Signatures...)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return false, fmt.Errorf("failed to add signatures: %v", err)
	}

	// If the number of signatures is less than the threshold, the transaction is pending
	return sigCount >= int(page.Threshold), nil
}

func (m *Executor) validateAgainstLite(st *StateManager, env *transactions.Envelope, fee protocol.Fee) error {
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

	if !account.DebitCredits(uint64(fee)) {
		return fmt.Errorf("insufficent credits for the transaction: %q has %v, cost is %d", account.Url, account.CreditBalance.String(), fee)
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
		m.logError("Failed to store transaction", "txid", logging.AsHex(env.GetTxHash()), "origin", env.Transaction.Origin, "error", err)
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

	// Update the account's list of pending transactions
	pending := indexing.PendingTransactions(m.blockBatch, st.OriginUrl)
	if status.Pending {
		err := pending.Add(st.txHash)
		if err != nil {
			return fmt.Errorf("failed to add transaction to the pending list: %v", err)
		}
	} else if status.Delivered {
		err := pending.Remove(st.txHash)
		if err != nil {
			return fmt.Errorf("failed to remove transaction to the pending list: %v", err)
		}
	}

	// Add the transaction to the origin's main chain, unless it's pending
	if !status.Pending {
		err = addChainEntry(m.Network.NodeUrl(), m.blockBatch, st.OriginUrl, protocol.MainChain, protocol.ChainTypeTransaction, env.GetTxHash(), 0, 0)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("failed to add signature to pending chain: %v", err)
		}
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
	if err != nil || fee > protocol.FeeFailedMaximum {
		fee = protocol.FeeFailedMaximum
	}
	st.Signator.DebitCredits(uint64(fee))

	return sigRecord.PutState(st.Signator)
}
