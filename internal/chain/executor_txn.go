package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// CheckTx implements ./abci.Chain
func (m *Executor) CheckTx(env *protocol.Envelope) (protocol.TransactionResult, *protocol.Error) {
	batch := m.DB.Begin(false)
	defer batch.Discard()

	isInitial := env.Transaction != nil && env.Transaction.Body.Type() != protocol.TransactionTypeSignPending
	transaction, err := m.ValidateEnvelope(batch, env)
	if err != nil {
		return nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}

	env.Transaction = transaction
	st, _, err := m.validate(nil, batch, env, true)
	var notFound bool
	switch {
	case err == nil:
		// OK
	case errors.Is(err, storage.ErrNotFound):
		notFound = true
	default:
		return nil, &protocol.Error{Code: protocol.ErrorCodeCheckTxError, Message: err}
	}
	defer st.Discard()

	// Do not run transaction-specific validation for a synthetic transaction. A
	// synthetic transaction will be rejected by `m.validate` unless it is
	// signed by a BVN and can be proved to have been included in a DN block. If
	// `m.validate` succeeeds, we know the transaction came from a BVN, thus it
	// is safe and reasonable to allow the transaction to be delivered.
	//
	// This is important because if a synthetic transaction is rejected during
	// CheckTx, it does not get recorded. If the synthetic transaction is not
	// recorded, the BVN that sent it and the client that sent the original
	// transaction cannot verify that the synthetic transaction was received.
	if st.txType.IsSynthetic() {
		return new(protocol.EmptyResult), nil
	}

	// Synthetic transactions with a missing origin should still be recorded.
	// Thus this check only happens if the transaction is not synthetic.
	if notFound {
		return nil, &protocol.Error{Code: protocol.ErrorCodeNotFound, Message: err}
	}

	// Only validate the transaction when we first receive it
	if isInitial {
		return new(protocol.EmptyResult), nil
	}

	executor, ok := m.executors[st.txType]
	if !ok {
		// This should never happen
		return nil, protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", st.txType)
	}

	result, err := executor.Validate(st, env)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeValidateTxnError, Message: err}
	}
	if result == nil {
		result = new(protocol.EmptyResult)
	}
	return result, nil
}

// DeliverTx implements ./abci.Chain
func (m *Executor) DeliverTx(env *protocol.Envelope) (protocol.TransactionResult, *protocol.Error) {
	// if txt.IsInternal() && tx.Transaction.Nonce != uint64(m.blockIndex) {
	// 	err := fmt.Errorf("nonce does not match block index, want %d, got %d", m.blockIndex, tx.Transaction.Nonce)
	// 	return nil, m.recordTransactionError(tx, nil, nil, &chainId, tx.GetTxHash(), &protocol.Error{Code: protocol.CodeInvalidTxnError, Message: err})
	// }

	batch := m.blockBatch.Begin()
	defer batch.Discard()

	// Validate the envelope, and load the transaction if necessary
	transaction, err := m.ValidateEnvelope(batch, env)
	if err != nil {
		return nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
	}

	// Process each signature
	for _, signature := range env.Signatures {
		err = m.ProcessSignature(batch, transaction, signature)
		if err != nil {
			return nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
		}
	}

	env.Transaction = transaction

	// Set up the state manager and validate the signatures
	st, hasEnoughSigs, err := m.validate(m.blockBatch, batch, env, false)
	if err != nil {
		return nil, m.recordTransactionError(nil, env, false, &protocol.Error{Code: protocol.ErrorCodeCheckTxError, Message: fmt.Errorf("txn check failed : %v", err)})
	}
	defer st.Discard()

	if !hasEnoughSigs {
		// Write out changes to the nonce and credit balance
		err = st.Commit()
		if err != nil {
			return nil, m.recordTransactionError(st, env, true, &protocol.Error{Code: protocol.ErrorCodeRecordTxnError, Message: err})
		}

		status := &protocol.TransactionStatus{Pending: true}
		err = m.putTransaction(st, env, status, false)
		if err != nil {
			return nil, &protocol.Error{Code: protocol.ErrorCodeTxnStateError, Message: err}
		}
		return new(protocol.EmptyResult), nil
	}

	executor, ok := m.executors[st.txType]
	if !ok {
		// This should never happen
		return nil, protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", st.txType)
	}

	result, err := executor.Validate(st, env)
	if err != nil {
		return nil, m.recordTransactionError(st, env, false, &protocol.Error{Code: protocol.ErrorCodeInvalidTxnError, Message: fmt.Errorf("txn validation failed : %v", err)})
	}
	if result == nil {
		result = new(protocol.EmptyResult)
	}

	// Store pending state updates, queue state creates for synthetic transactions
	err = st.Commit()
	if err != nil {
		return nil, m.recordTransactionError(st, env, true, &protocol.Error{Code: protocol.ErrorCodeRecordTxnError, Message: err})
	}

	// Store the tx state
	status := &protocol.TransactionStatus{Delivered: true, Result: result}
	err = m.putTransaction(st, env, status, false)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeTxnStateError, Message: err}
	}

	m.blockState.Merge(&st.blockState)

	// Process synthetic transactions generated by the validator
	produced := st.blockState.ProducedTxns
	st.Reset()
	err = m.addSynthTxns(&st.stateCache, produced)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeSyntheticTxnError, Message: err}
	}
	err = st.Commit()
	if err != nil {
		return nil, m.recordTransactionError(st, env, true, &protocol.Error{Code: protocol.ErrorCodeRecordTxnError, Message: err})
	}

	m.blockState.Delivered++
	return result, nil
}

func (m *Executor) processInternalDataTransaction(internalAccountPath string, wd *protocol.WriteData) error {
	dataAccountUrl := m.Network.NodeUrl(internalAccountPath)

	if wd == nil {
		return fmt.Errorf("no internal data transaction provided")
	}

	signer := m.Network.ValidatorPage(0)
	env := new(protocol.Envelope)
	env.Transaction = new(protocol.Transaction)
	env.Transaction.Header.Principal = m.Network.NodeUrl()
	env.Transaction.Body = wd
	env.Transaction.Header.Initiator = signer.AccountID32()
	env.Signatures = []protocol.Signature{&protocol.InternalSignature{Network: signer}}

	sw := protocol.SegWitDataEntry{}
	sw.Cause = *(*[32]byte)(env.GetTxHash())
	sw.EntryHash = *(*[32]byte)(wd.Entry.Hash())
	sw.EntryUrl = env.Transaction.Header.Principal
	env.Transaction.Body = &sw

	st, err := NewStateManager(m.blockBatch, nil, m.Network.NodeUrl(internalAccountPath), env)
	if err != nil {
		return err
	}
	defer st.Discard()
	st.logger.L = m.logger

	var da *protocol.DataAccount
	va := m.blockBatch.Account(dataAccountUrl)
	err = va.GetStateAs(&da)
	if err != nil {
		return err
	}

	st.UpdateData(da, wd.Entry.Hash(), &wd.Entry)

	status := &protocol.TransactionStatus{Delivered: true}
	err = m.blockBatch.Transaction(env.GetTxHash()).Put(env, status, nil)
	if err != nil {
		return err
	}

	err = st.Commit()
	return err
}

// validate validates signatures, verifies they are authorized,
// updates the nonce, and charges the fee.
func (m *Executor) validate(parentBatch *database.Batch, batch *database.Batch, env *protocol.Envelope, isCheck bool) (st *StateManager, hasEnoughSigs bool, err error) {
	// Set up the state manager
	st, err = NewStateManager(parentBatch, batch, m.Network.NodeUrl(), env)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if st != nil && err != nil {
			st.Discard()
		}
	}()
	if isCheck {
		st.logger.L = m.logger.With("operation", "Check")
	} else {
		st.logger.L = m.logger.With("operation", "Deliver")
	}

	// Validate the transaction
	switch {
	case env.Transaction.Body.Type().IsUser():
		hasEnoughSigs, err = m.validateUser(st, env, isCheck)

	case env.Transaction.Body.Type().IsSynthetic():
		hasEnoughSigs, err = true, m.validateSynthetic(st, env)

	case env.Transaction.Body.Type().IsInternal():
		hasEnoughSigs, err = true, nil

	default:
		return nil, false, fmt.Errorf("invalid transaction type %v", env.Transaction.Body.Type())
	}

	switch {
	case err == nil:
		return st, hasEnoughSigs, err
	case errors.Is(err, storage.ErrNotFound):
		return st, hasEnoughSigs, err
	default:
		return nil, false, err
	}
}

func (m *Executor) validateUser(st *StateManager, env *protocol.Envelope, isCheck bool) (bool, error) {
	if isCheck {
		return false, nil
	}

	page, ok := st.Signator.(*protocol.KeyPage)
	if !ok {
		return true, nil
	}

	sigs, err := st.LoadSignatures(*(*[32]byte)(env.GetTxHash()))
	if err != nil {
		return false, err
	}

	return page.Threshold <= uint64(sigs.Count()), nil
}

func (m *Executor) validateSynthetic(st *StateManager, env *protocol.Envelope) error {
	// Does the origin exist?
	if st.Origin != nil {
		return nil
	}

	// Is it OK for the origin to be missing?
	switch st.txType {
	case protocol.TransactionTypeSyntheticCreateChain,
		protocol.TransactionTypeSyntheticDepositTokens,
		protocol.TransactionTypeSyntheticWriteData:
		// These transactions allow for a missing origin
		return nil

	default:
		return fmt.Errorf("origin %q %w", st.OriginUrl, storage.ErrNotFound)
	}
}

func (m *Executor) recordTransactionError(st *StateManager, env *protocol.Envelope, postCommit bool, failure *protocol.Error) *protocol.Error {
	status := &protocol.TransactionStatus{
		Delivered: true,
		Code:      uint64(failure.Code),
		Message:   failure.Error(),
	}
	err := m.putTransaction(st, env, status, postCommit)
	if err != nil {
		m.logError("Failed to store transaction", "txid", logging.AsHex(env.GetTxHash()), "origin", env.Transaction.Header.Principal, "error", err)
	}
	return failure
}

func (m *Executor) putTransaction(st *StateManager, env *protocol.Envelope, status *protocol.TransactionStatus, postCommit bool) (err error) {
	// Don't add internal transactions to chains. Internal transactions are
	// exclusively used for communication between the governor and the state
	// machine.
	txt := env.Transaction.Type()
	if txt.IsInternal() {
		return nil
	}

	// Store against the transaction hash
	stateEnv := new(protocol.Envelope)
	stateEnv.Transaction = env.Transaction
	err = m.blockBatch.Transaction(env.GetTxHash()).Put(stateEnv, status, env.Signatures)
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

	block := new(BlockState)
	defer func() {
		if err == nil {
			m.blockState.Merge(block)
		}
	}()

	// Add the transaction to the origin's main chain, unless it's pending
	if !status.Pending {
		err = addChainEntry(block, m.blockBatch, st.OriginUrl, protocol.MainChain, protocol.ChainTypeTransaction, env.GetTxHash(), 0, 0)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("failed to add signature to pending chain: %v", err)
		}
	}

	// The signator of a synthetic transaction is the subnet ADI (acc://dn or
	// acc://bvn-{id}). Do not charge fees to the subnet's ADI.
	if txt.IsSynthetic() {
		return nil
	}

	// If the transaction failed, issue a refund
	if status.Code == protocol.ErrorCodeOK.GetEnumValue() {
		return nil
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := protocol.ComputeTransactionFee(env.Transaction)
	if err != nil || paid <= protocol.FeeFailedMaximum {
		return nil
	}

	sigRecord := m.blockBatch.Account(st.SignatorUrl)
	err = sigRecord.GetStateAs(&st.Signator)
	if err != nil {
		return err
	}

	refund := paid - protocol.FeeFailedMaximum
	st.Signator.CreditCredits(refund.AsUInt64())
	return sigRecord.PutState(st.Signator)
}
