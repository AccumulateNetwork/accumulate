package block

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (*Executor) LoadTransaction(batch *database.Batch, envelope *protocol.Envelope) (*protocol.Transaction, error) {
	// An envelope with no signatures is invalid
	if len(envelope.Signatures) == 0 {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "envelope has no signatures")
	}

	// The transaction hash must be specified for signature transactions
	if len(envelope.TxHash) == 0 && envelope.Type() == protocol.TransactionTypeSignPending {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "cannot sign pending transaction: missing transaction hash")
	}

	// The transaction hash and/or the transaction itself must be specified
	if len(envelope.TxHash) == 0 && envelope.Transaction == nil {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "envelope has neither transaction nor hash")
	}

	// The transaction hash must be the correct size
	if len(envelope.TxHash) > 0 && len(envelope.TxHash) != sha256.Size {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "transaction hash is the wrong size")
	}

	// If a transaction and a hash are specified, they must match
	if !envelope.VerifyTxHash() {
		return nil, protocol.Errorf(protocol.ErrorCodeInvalidRequest, "transaction hash does not match transaction")
	}

	// Check the transaction status
	status, err := batch.Transaction(envelope.GetTxHash()).GetStatus()
	switch {
	case err != nil:
		// Unknown error
		return nil, fmt.Errorf("load transaction status: %w", err)

	case status.Delivered:
		// Transaction has already been delivered
		return nil, protocol.Errorf(protocol.ErrorCodeAlreadyDelivered, "transaction has already been delivered")
	}

	// Ignore produced synthetic transactions
	if status.Remote && !status.Pending {
		return envelope.Transaction, nil
	}

	// Load previous transaction state
	txState, err := batch.Transaction(envelope.GetTxHash()).GetState()
	switch {
	case err == nil:
		// Load existing the transaction from the database
		return txState.Transaction, nil

	case !errors.Is(err, storage.ErrNotFound):
		// Unknown error
		return nil, fmt.Errorf("load transaction: %v", err)

	case envelope.Type() == protocol.TransactionTypeSignPending:
		// If the envelope does not include the transaction, it must exist
		// in the database
		return nil, fmt.Errorf("load transaction: %v", err)

	default:
		// Transaction is new
		return envelope.Transaction, nil
	}
}

// ProcessTransaction processes a transaction. It will not return an error if
// the transaction fails - in that case the status code will be non zero. It
// only returns an error in cases like a database failure.
func (x *Executor) ProcessTransaction(batch *database.Batch, transaction *protocol.Transaction) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Load the status
	status, err := batch.Transaction(transaction.GetHash()).GetStatus()
	if err != nil {
		return nil, nil, err
	}
	if status.Initiator == nil {
		// This should never happen
		return nil, nil, fmt.Errorf("transaction initiator is missing")
	}

	// Load the initiator
	var signer protocol.Signer
	err = batch.Account(status.Initiator).GetStateAs(&signer)
	if err != nil {
		err = fmt.Errorf("load initiator: %w", err)
		return recordFailedTransaction(batch, transaction, nil, err)
	}

	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, storage.ErrNotFound):
		err = fmt.Errorf("load principal: %w", err)
		return recordFailedTransaction(batch, transaction, signer, err)
	case !transactionAllowsMissingPrincipal(transaction):
		err = fmt.Errorf("load principal: %w", err)
		return recordFailedTransaction(batch, transaction, signer, err)
	}

	// Check if the transaction is ready to be executed
	ready, err := TransactionIsReady(batch, transaction, status)
	if err != nil {
		return recordFailedTransaction(batch, transaction, signer, err)
	}
	if !ready {
		return recordPendingTransaction(batch, transaction)
	}

	if transaction.Body.Type().IsSynthetic() {
		// Verify that the synthetic transaction has all the right signatures
		err = processSyntheticTransaction(&x.Network, batch, transaction, status)
		if err != nil {
			return recordFailedTransaction(batch, transaction, signer, err)
		}
	}

	// Set up the state manager
	st := chain.NewStateManager(batch.Begin(true), x.Network.NodeUrl(), signer.GetUrl(), signer, principal, transaction, x.logger.With("operation", "ProcessTransaction"))
	defer st.Discard()

	// Execute the transaction
	executor, ok := x.executors[transaction.Body.Type()]
	if !ok {
		// An invalid transaction should not make it to this point
		err = protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", transaction.Body.Type())
		return recordFailedTransaction(batch, transaction, signer, err)
	}

	result, err := executor.Execute(st, &protocol.Envelope{Transaction: transaction})
	if err != nil {
		return recordFailedTransaction(batch, transaction, signer, err)
	}

	// Commit changes, queue state creates for synthetic transactions
	state, err := st.Commit()
	if err != nil {
		err = fmt.Errorf("commit: %w", err)
		return recordFailedTransaction(batch, transaction, signer, err)
	}

	return recordSuccessfulTransaction(batch, state, transaction, result)
}

func transactionAllowsMissingPrincipal(transaction *protocol.Transaction) bool {
	switch transaction.Body.Type() {
	case protocol.TransactionTypeSyntheticCreateChain,
		protocol.TransactionTypeSyntheticDepositTokens,
		protocol.TransactionTypeSyntheticWriteData:
		// These transactions allow for a missing origin
		return true
	default:
		return false
	}
}

func TransactionIsReady(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (bool, error) {
	// TODO When we implement 'pending' synthetic transactions, this is where
	// the logic will go

	if !transaction.Body.Type().IsUser() {
		return true, nil
	}

	// UpdateKey transactions are always M=1 and always require a signature from
	// the initiator
	txnObj := batch.Transaction(transaction.GetHash())
	if transaction.Body.Type() == protocol.TransactionTypeUpdateKey {
		if status.Initiator == nil {
			return false, fmt.Errorf("missing initiator")
		}

		initSigs, err := txnObj.ReadSignatures(status.Initiator)
		if err != nil {
			return false, fmt.Errorf("load initiator signatures: %w", err)
		}

		if initSigs.Count() == 0 {
			return false, fmt.Errorf("missing initiator signature")
		}

		return true, nil
	}

	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return false, fmt.Errorf("load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := getAccountAuth(batch, principal)
	if err != nil {
		return false, fmt.Errorf("unable to load authority of %v: %w", transaction.Header.Principal, err)
	}

	// For each authority
outer:
	for _, entry := range auth.Authorities {
		// Do not check signers for disabled authorities
		if entry.Disabled {
			continue
		}

		// Check if any signer has reached its threshold
		for _, signerUrl := range status.FindSigners(entry.Url) {
			// TODO Enable remote signers
			if !principal.GetUrl().RootIdentity().Equal(signerUrl.RootIdentity()) {
				continue
			}

			// Load the signer
			var signer protocol.Signer
			err := batch.Account(signerUrl).GetStateAs(&signer)
			if err != nil {
				return false, fmt.Errorf("load signer %v: %w", signerUrl, err)
			}

			// Load the signature set
			signatures, err := txnObj.ReadSignatures(signerUrl)
			if err != nil {
				return false, fmt.Errorf("load signatures set %v: %w", signerUrl, err)
			}

			// Check if the threshold has been reached
			if uint64(signatures.Count()) >= signer.GetSignatureThreshold() {
				continue outer
			}
		}

		return false, nil
	}

	// If every authority is disabled, at least one signature is required
	return len(status.Signers) > 0, nil
}

func recordTransaction(batch *database.Batch, transaction *protocol.Transaction, updateStatus func(*protocol.TransactionStatus)) (*protocol.TransactionStatus, error) {
	// Store the transaction state (without signatures)
	stateEnv := new(protocol.Envelope)
	stateEnv.Transaction = transaction
	db := batch.Transaction(transaction.GetHash())
	err := db.PutState(stateEnv)
	if err != nil {
		return nil, fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	status, err := db.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction status: %w", err)
	}

	updateStatus(status)
	err = db.PutStatus(status)
	if err != nil {
		return nil, fmt.Errorf("store transaction status: %w", err)
	}

	return status, nil
}

func recordPendingTransaction(batch *database.Batch, transaction *protocol.Transaction) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		status.Remote = false
		status.Pending = true
	})
	if err != nil {
		return nil, nil, err
	}

	// Add the transaction to the principal's list of pending transactions
	pending := indexing.PendingTransactions(batch, transaction.Header.Principal)
	err = pending.Add(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return nil, nil, fmt.Errorf("store pending list: %w", err)
	}

	return status, new(chain.ProcessTransactionState), nil
}

func recordSuccessfulTransaction(batch *database.Batch, state *chain.ProcessTransactionState, transaction *protocol.Transaction, result protocol.TransactionResult) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		status.Remote = false
		status.Pending = false
		status.Delivered = true
		status.Code = 0
		if result == nil {
			status.Result = new(protocol.EmptyResult)
		} else {
			status.Result = result
		}
	})
	if err != nil {
		return nil, nil, err
	}

	// Create receipt
	if chain.NeedsReceipt(transaction.Body.Type()) {
		state.DidProduceTxn(chain.CreateSynthReceipt(transaction, status))
	}

	// Don't add internal transactions to chains
	if transaction.Body.Type().IsInternal() {
		return status, state, nil
	}

	// Remove the transaction from the principal's list of pending transactions
	pending := indexing.PendingTransactions(batch, transaction.Header.Principal)
	err = pending.Remove(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return nil, nil, fmt.Errorf("store pending list: %w", err)
	}

	// Add the transaction to the principal's main chain
	err = state.ChainUpdates.AddChainEntry(batch, transaction.Header.Principal, protocol.MainChain, protocol.ChainTypeTransaction, transaction.GetHash(), 0, 0)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, fmt.Errorf("add to chain: %v", err)
	}

	return status, state, nil
}

func recordFailedTransaction(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, failure error) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		failure := protocol.NewError(protocol.ErrorCodeUnknownError, failure)
		status.Remote = false
		status.Delivered = true
		status.Code = failure.Code.GetEnumValue()
		status.Message = failure.Error()
	})
	if err != nil {
		return nil, nil, err
	}

	// Create receipt
	state := new(chain.ProcessTransactionState)
	if chain.NeedsReceipt(transaction.Body.Type()) {
		state.DidProduceTxn(chain.CreateSynthReceipt(transaction, status))
	}

	// Remove the transaction from the principal's list of pending transactions
	pending := indexing.PendingTransactions(batch, transaction.Header.Principal)
	err = pending.Remove(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return nil, nil, fmt.Errorf("update pending list: %w", err)
	}

	// Refund the signer
	if signer == nil || !transaction.Body.Type().IsUser() {
		return status, state, nil
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := protocol.ComputeTransactionFee(transaction)
	if err != nil || paid <= protocol.FeeFailedMaximum {
		return nil, nil, fmt.Errorf("compute fee: %w", err)
	}

	refund := paid - protocol.FeeFailedMaximum
	signer.CreditCredits(refund.AsUInt64())
	err = batch.Account(signer.GetUrl()).PutState(signer)
	if err != nil {
		return nil, nil, fmt.Errorf("refund initial signer: %w", err)
	}

	return status, state, nil
}
