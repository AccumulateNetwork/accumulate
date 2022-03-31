package chain

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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

func (x *Executor) ProcessTransaction(batch *database.Batch, transaction *protocol.Transaction) (protocol.TransactionResult, *BlockState, error) {
	// Load the signatures
	signatures, err := batch.Transaction(transaction.GetHash()).GetSignatures()
	if err != nil {
		return nil, nil, err
	}

	// Load the first signer
	firstSig := signatures.Signatures[0]
	if _, ok := firstSig.(*protocol.ReceiptSignature); ok {
		err = protocol.Errorf(protocol.ErrorCodeInvalidRequest, "invalid transaction: initiated by receipt signature")
		x.recordFailedTransaction(batch, transaction, nil, err)
		return nil, nil, err
	}

	var signer protocol.SignerAccount
	err = batch.Account(firstSig.GetSigner()).GetStateAs(&signer)
	if err != nil {
		err = fmt.Errorf("load signer: %w", err)
		x.recordFailedTransaction(batch, transaction, nil, err)
		return nil, nil, err
	}

	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, storage.ErrNotFound):
		err = fmt.Errorf("load principal: %w", err)
		x.recordFailedTransaction(batch, transaction, signer, err)
		return nil, nil, err
	case !transactionAllowsMissingPrincipal(transaction):
		err = fmt.Errorf("load principal: %w", err)
		x.recordFailedTransaction(batch, transaction, signer, err)
		return nil, nil, err
	}

	// Check if the transaction is ready to be executed
	if !transactionIsReady(transaction, signer, signatures) {
		err = x.recordPendingTransaction(batch, transaction)
		if err != nil {
			x.logger.Error("Unable to record successful transaction", "txid", logging.AsHex(transaction.GetHash()), "origin", transaction.Header.Principal, "error", err)
			return nil, nil, err
		}
		return new(protocol.EmptyResult), new(BlockState), nil
	}

	if transaction.Body.Type().IsSynthetic() {
		// Verify that the synthetic transaction has all the right signatures
		err = processSyntheticTransaction(&x.Network, batch, transaction, signatures.Signatures)
		if err != nil {
			x.recordFailedTransaction(batch, transaction, signer, err)
			return nil, nil, err
		}
	}

	// Set up the state manager
	st := NewStateManager(batch.Begin(true), x.Network.NodeUrl(), signer.Header().Url, signer, principal, transaction)
	defer st.Discard()
	st.logger.L = x.logger.With("operation", "ProcessTransaction")

	// Execute the transaction
	executor, ok := x.executors[transaction.Body.Type()]
	if !ok {
		// An invalid transaction should not make it to this point
		err = protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", transaction.Body.Type())
		x.recordFailedTransaction(batch, transaction, signer, err)
		return nil, nil, err
	}

	result, err := executor.Validate(st, &protocol.Envelope{Transaction: transaction})
	if err != nil {
		st.stateCache.blockState.ProducedTxns = make([]*protocol.Transaction, 0) // Clear synth txs if any were produced, we only want to keep SyntheticReceipt
		x.recordFailedTransaction(batch, transaction, signer, err)
		return nil, nil, err
	}

	// Commit changes, queue state creates for synthetic transactions
	err = st.Commit()
	if err != nil {
		err = fmt.Errorf("commit: %w", err)
		x.recordFailedTransaction(batch, transaction, signer, err)
		return nil, nil, err
	}

	blockState, err := x.recordSuccessfulTransaction(batch, transaction, result)
	if err != nil {
		x.logger.Error("Unable to record successful transaction", "txid", logging.AsHex(transaction.GetHash()), "origin", transaction.Header.Principal, "error", err)
		return nil, nil, err
	}

	// Submit SyntheticReceipt to the current state manager/batch so ProduceSynthetic doesn't have to create a new separate one
	srEnv := x.blockState.SynthReceiptEnvelope
	if srEnv != nil {
		x.blockState.SynthReceiptEnvelope = nil
		st.Submit(srEnv.DestUrl, srEnv.SyntheticReceipt)
	}

	st.blockState.Merge(blockState)
	st.blockState.Delivered = 1
	return result, &st.blockState, nil
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

func transactionIsReady(transaction *protocol.Transaction, signer protocol.SignerAccount, signatures *database.SignatureSet) bool {
	// TODO When we implement 'pending' synthetic transactions, this is where
	// the logic will go

	if !transaction.Body.Type().IsUser() {
		return true
	}

	return uint64(signatures.Count()) >= signer.GetSignatureThreshold()
}

func (x *Executor) recordTransaction(batch *database.Batch, transaction *protocol.Transaction, updateStatus func(*protocol.TransactionStatus)) error {
	// Store the transaction state (without signatures)
	stateEnv := new(protocol.Envelope)
	stateEnv.Transaction = transaction
	db := batch.Transaction(transaction.GetHash())
	err := db.PutState(stateEnv)
	if err != nil {
		return fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	status, err := db.GetStatus()
	if err != nil {
		return fmt.Errorf("load transaction status: %w", err)
	}

	updateStatus(status)
	err = db.PutStatus(status)
	if err != nil {
		return fmt.Errorf("store transaction status: %w", err)
	}

	// When the transaction is synthetic, send a receipt back to its origin
	txt := stateEnv.Transaction.Type()
	if txt.IsSynthetic() && NeedsReceipt(txt) { // recordTransactionError can pass in a nil state manager
		if x.blockState.SynthReceiptEnvelope != nil {
			panic("BlockState can hold only one SynthReceiptEnvelope")
		}
		x.blockState.SynthReceiptEnvelope = CreateSynthReceipt(stateEnv, status, x.Network.NodeUrl())
	}
	return nil
}

func (x *Executor) recordPendingTransaction(batch *database.Batch, transaction *protocol.Transaction) error {
	// Record the transaction
	err := x.recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		status.Remote = false
		status.Pending = true
	})
	if err != nil {
		return err
	}

	// Add the transaction to the principal's list of pending transactions
	pending := indexing.PendingTransactions(batch, transaction.Header.Principal)
	err = pending.Add(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return fmt.Errorf("store pending list: %w", err)
	}

	return nil
}

func (x *Executor) recordSuccessfulTransaction(batch *database.Batch, transaction *protocol.Transaction, result protocol.TransactionResult) (*BlockState, error) {
	// Record the transaction
	err := x.recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
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
		return nil, err
	}

	// Don't add internal transactions to chains
	if transaction.Body.Type().IsInternal() {
		return new(BlockState), nil
	}

	// Remove the transaction from the principal's list of pending transactions
	pending := indexing.PendingTransactions(batch, transaction.Header.Principal)
	err = pending.Remove(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return nil, fmt.Errorf("store pending list: %w", err)
	}

	// Add the transaction to the principal's main chain
	block := new(BlockState)
	err = addChainEntry(block, batch, transaction.Header.Principal, protocol.MainChain, protocol.ChainTypeTransaction, transaction.GetHash(), 0, 0)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("add to chain: %v", err)
	}

	return block, nil
}

func (x *Executor) recordFailedTransaction(batch *database.Batch, transaction *protocol.Transaction, signer protocol.SignerAccount, failure error) {
	// Record the transaction
	err := x.recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		failure := protocol.NewError(protocol.ErrorCodeUnknownError, failure)
		status.Remote = false
		status.Delivered = true
		status.Code = failure.Code.GetEnumValue()
		status.Message = failure.Error()
	})
	if err != nil {
		x.logger.Error("Unable to record failed transaction", "txid", logging.AsHex(transaction.GetHash()), "origin", transaction.Header.Principal, "error", err)
		return
	}

	// Remove the transaction from the principal's list of pending transactions
	pending := indexing.PendingTransactions(batch, transaction.Header.Principal)
	err = pending.Remove(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		x.logger.Error("Unable to record failed transaction - update pending list", "txid", logging.AsHex(transaction.GetHash()), "origin", transaction.Header.Principal, "error", err)
		return
	}

	// Refund the signer
	if signer == nil || !transaction.Body.Type().IsUser() {
		return
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := protocol.ComputeTransactionFee(transaction)
	if err != nil || paid <= protocol.FeeFailedMaximum {
		x.logger.Error("Unable to record failed transaction - calculate fee", "txid", logging.AsHex(transaction.GetHash()), "origin", transaction.Header.Principal, "error", err)
		return
	}

	refund := paid - protocol.FeeFailedMaximum
	signer.CreditCredits(refund.AsUInt64())
	err = batch.Account(signer.Header().Url).PutState(signer)
	if err != nil {
		x.logger.Error("Unable to record failed transaction - refund initial signer", "txid", logging.AsHex(transaction.GetHash()), "origin", transaction.Header.Principal, "error", err)
		return
	}
}
