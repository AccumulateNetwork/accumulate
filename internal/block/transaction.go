package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

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

	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, storage.ErrNotFound):
		err = errors.Format(errors.StatusUnknown, "load principal: %w", err)
		return x.recordFailedTransaction(batch, transaction, err)
	case !x.transactionAllowsMissingPrincipal(transaction):
		err = errors.Format(errors.StatusUnknown, "load principal: %w", err)
		return x.recordFailedTransaction(batch, transaction, err)
	}

	// Check if the transaction is ready to be executed
	ready, err := x.TransactionIsReady(batch, transaction, status)
	if err != nil {
		return x.recordFailedTransaction(batch, transaction, err)
	}
	if !ready {
		return recordPendingTransaction(&x.Network, batch, transaction)
	}

	if transaction.Body.Type().IsSynthetic() {
		// Verify that the synthetic transaction has all the right signatures
		err = processSyntheticTransaction(batch, transaction, status)
		if err != nil {
			return x.recordFailedTransaction(batch, transaction, err)
		}
	}

	// Set up the state manager
	st, err := chain.LoadStateManager(&x.Network, batch.Begin(true), principal, transaction, status, x.logger.With("operation", "ProcessTransaction"))
	if err != nil {
		return x.recordFailedTransaction(batch, transaction, err)
	}
	defer st.Discard()

	// Execute the transaction
	executor, ok := x.executors[transaction.Body.Type()]
	if !ok {
		// An invalid transaction should not make it to this point
		err = protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", transaction.Body.Type())
		return x.recordFailedTransaction(batch, transaction, err)
	}

	result, err := executor.Execute(st, &chain.Delivery{Transaction: transaction})
	if err != nil {
		err = errors.Wrap(0, err)
		return x.recordFailedTransaction(batch, transaction, err)
	}

	// Commit changes, queue state creates for synthetic transactions
	state, err := st.Commit()
	if err != nil {
		err = fmt.Errorf("commit: %w", err)
		return x.recordFailedTransaction(batch, transaction, err)
	}

	return recordSuccessfulTransaction(batch, state, transaction, result)
}

func (x *Executor) transactionAllowsMissingPrincipal(transaction *protocol.Transaction) bool {
	val, ok := getValidator[chain.PrincipalValidator](x, transaction.Body.Type())
	if ok {
		allow, fallback := val.AllowMissingPrincipal(transaction)
		if !fallback {
			return allow
		}
	}

	// TODO Replace with AllowMissingPrincipal
	switch body := transaction.Body.(type) {
	case *protocol.WriteData,
		*protocol.SyntheticWriteData:
		// WriteData and SyntheticWriteData can create a lite data account
		_, err := protocol.ParseLiteDataAddress(transaction.Header.Principal)
		return err == nil

	case *protocol.SyntheticDepositTokens:
		// SyntheticDepositTokens can create a lite token account
		key, _, _ := protocol.ParseLiteTokenAddress(transaction.Header.Principal)
		return key != nil

	case *protocol.SyntheticCreateIdentity:
		// SyntheticCreateChain can create accounts
		return true

	case *protocol.SyntheticForwardTransaction:
		return x.transactionAllowsMissingPrincipal(body.Transaction)

	default:
		return false
	}
}

func (x *Executor) TransactionIsReady(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (bool, error) {
	switch {
	case transaction.Body.Type().IsUser():
		return x.userTransactionIsReady(batch, transaction, status)
	case transaction.Body.Type().IsSynthetic():
		return x.synthTransactionIsReady(batch, transaction, status)
	default:
		return true, nil
	}
}

func (x *Executor) userTransactionIsReady(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (bool, error) {
	// UpdateKey transactions are always M=1 and always require a signature from
	// the initiator
	if transaction.Body.Type() == protocol.TransactionTypeUpdateKey {
		if status.Initiator == nil {
			return false, fmt.Errorf("missing initiator")
		}

		initSigs, err := batch.Transaction(transaction.GetHash()).ReadSignatures(status.Initiator)
		if err != nil {
			return false, fmt.Errorf("load initiator signatures: %w", err)
		}

		if initSigs.Count() == 0 {
			return false, fmt.Errorf("missing initiator signature")
		}

		return true, nil
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](x, transaction.Body.Type())
	if ok {
		ready, fallback, err := val.TransactionIsReady(x, batch, transaction, status)
		if err != nil {
			return false, errors.Wrap(errors.StatusUnknown, err)
		}
		if !fallback {
			return ready, nil
		}
	}

	// Load the principal
	principal, err := batch.Account(transaction.Header.Principal).GetState()
	if err != nil {
		return false, errors.Format(errors.StatusUnknown, "load principal: %w", err)
	}

	// Get the principal's account auth
	auth, err := x.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return false, fmt.Errorf("unable to load authority of %v: %w", transaction.Header.Principal, err)
	}

	// For each authority
	for _, entry := range auth.Authorities {
		// Do not check signers for disabled authorities
		if entry.Disabled {
			continue
		}

		// Check if any signer has reached its threshold
		ok, err := x.AuthorityIsSatisfied(batch, transaction, status, entry.Url)
		if err != nil {
			return false, errors.Wrap(errors.StatusUnknown, err)
		}
		if !ok {
			return false, nil
		}
	}

	// If every authority is disabled, at least one signature is required
	return len(status.Signers) > 0, nil
}

func (x *Executor) AuthorityIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, authUrl *url.URL) (bool, error) {
	// Check if any signer has reached its threshold
	for _, signer := range status.FindSigners(authUrl) {
		ok, err := x.SignerIsSatisfied(batch, transaction, status, signer)
		if err != nil {
			return false, errors.Wrap(errors.StatusUnknown, err)
		}
		if ok {
			return true, nil
		}
	}

	return false, nil
}

func (x *Executor) SignerIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, signer protocol.Signer) (bool, error) {
	// Load the signature set
	signatures, err := batch.Transaction(transaction.GetHash()).ReadSignaturesForSigner(signer)
	if err != nil {
		return false, fmt.Errorf("load signatures set %v: %w", signer.GetUrl(), err)
	}

	// Check if the signature set includes a completed set
	for _, e := range signatures.Entries() {
		if e.Type == protocol.SignatureTypeSet {
			return true, nil
		}
	}

	// Check if the threshold has been reached
	if uint64(signatures.Count()) >= signer.GetSignatureThreshold() {
		return true, nil
	}

	return false, nil
}

func (x *Executor) synthTransactionIsReady(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (bool, error) {
	// Anchors cannot be pending
	if transaction.Body.Type() == protocol.TransactionTypeDirectoryAnchor || transaction.Body.Type() == protocol.TransactionTypePartitionAnchor {
		return true, nil
	}

	// Load all of the signatures
	signatures, err := GetAllSignatures(batch, batch.Transaction(transaction.GetHash()), status, transaction.Header.Initiator[:])
	if err != nil {
		return false, errors.Wrap(errors.StatusUnknown, err)
	}

	// Build a receipt from the signatures
	receipt, sourceNet, err := assembleSynthReceipt(transaction, signatures)
	if err != nil {
		return false, errors.Wrap(errors.StatusUnknown, err)
	}
	if receipt == nil {
		return false, nil
	}

	// Determine which anchor chain to load
	var subnet string
	if x.Network.Type != config.Directory {
		subnet = protocol.Directory
	} else {
		var ok bool
		subnet, ok = protocol.ParseSubnetUrl(sourceNet)
		if !ok {
			return false, errors.Format(errors.StatusUnknown, "%v is not a valid subnet URL", sourceNet)
		}
	}

	// Load the anchor chain
	anchorChain, err := batch.Account(x.Network.AnchorPool()).ReadChain(protocol.AnchorChain(subnet))
	if err != nil {
		return false, errors.Format(errors.StatusUnknown, "load %s intermediate anchor chain: %w", subnet, err)
	}

	// Is the result a valid DN anchor?
	_, err = anchorChain.HeightOf(receipt.Result)
	switch {
	case err == nil:
		// Ready
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	default:
		return false, errors.Format(errors.StatusUnknown, "get height of entry %X of %s intermediate anchor chain: %w", receipt.Result[:4], subnet, err)
	}

	// Get the synthetic signature
	synthSig, ok := signatures[0].(*protocol.SyntheticSignature)
	if !ok {
		return false, errors.Format(errors.StatusInternalError, "missing synthetic signature")
	}

	// Load the ledger
	var ledger *protocol.SyntheticLedger
	err = batch.Account(x.Network.Synthetic()).GetStateAs(&ledger)
	if err != nil {
		return false, errors.Format(errors.StatusUnknown, "load synthetic transaction ledger: %w", err)
	}

	// Verify the sequence number
	subnetLedger := ledger.Subnet(synthSig.SourceNetwork)
	if subnetLedger.Received+1 != synthSig.SequenceNumber {
		// TODO Out of sequence synthetic transactions should be marked pending
		// and the source BVN should be queried to retrieve the missing
		// synthetic transactions

		// TODO If a synthetic transaction fails, the ledger update is
		// discarded, resulting in an incorrect 'out of sequence' error

		x.logger.Error("Out of sequence synthetic transaction",
			"hash", logging.AsHex(transaction.GetHash()).Slice(0, 4),
			"seq-got", synthSig.SequenceNumber,
			"seq-want", subnetLedger.Received+1,
			"source", synthSig.SourceNetwork,
			"destination", synthSig.DestinationNetwork,
			"type", transaction.Body.Type(),
			"hash", logging.AsHex(transaction.GetHash()).Slice(0, 4),
		)
	}

	// Update the received number
	if synthSig.SequenceNumber > subnetLedger.Received {
		subnetLedger.Received = synthSig.SequenceNumber
		err = batch.Account(x.Network.Synthetic()).PutState(ledger)
		if err != nil {
			return false, errors.Format(errors.StatusUnknown, "store synthetic transaction ledger: %w", err)
		}
	}

	return true, nil
}

func recordTransaction(batch *database.Batch, transaction *protocol.Transaction, updateStatus func(*protocol.TransactionStatus)) (*protocol.TransactionStatus, error) {
	// Store the transaction state (without signatures)
	db := batch.Transaction(transaction.GetHash())
	err := db.PutState(&database.SigOrTxn{Transaction: transaction})
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

func recordPendingTransaction(net *config.Network, batch *database.Batch, transaction *protocol.Transaction) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		status.Remote = false
		status.Pending = true
	})
	if err != nil {
		return nil, nil, err
	}

	// Add the user transaction to the principal's list of pending transactions
	if transaction.Body.Type().IsUser() {
		err = batch.Account(transaction.Header.Principal).AddPending(*(*[32]byte)(transaction.GetHash()))
		if err != nil {
			return nil, nil, fmt.Errorf("store pending list: %w", err)
		}

		return status, new(chain.ProcessTransactionState), nil
	}

	// Load all of the signatures
	signatures, err := GetAllSignatures(batch, batch.Transaction(transaction.GetHash()), status, transaction.Header.Initiator[:])
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Add the synthetic transaction to the anchor's list of pending transactions
	receipt, _, err := assembleSynthReceipt(transaction, signatures)
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	err = batch.Account(net.Ledger()).AddSyntheticForAnchor(*(*[32]byte)(receipt.Result), *(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
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

	// Don't add internal transactions to chains
	if transaction.Body.Type().IsSystem() {
		return status, state, nil
	}

	// Remove the transaction from the principal's list of pending transactions
	err = batch.Account(transaction.Header.Principal).RemovePending(*(*[32]byte)(transaction.GetHash()))
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

func (x *Executor) recordFailedTransaction(batch *database.Batch, transaction *protocol.Transaction, failure error) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := recordTransaction(batch, transaction, func(status *protocol.TransactionStatus) {
		status.Remote = false
		status.Pending = false
		status.Delivered = true
		status.Message = failure.Error()

		var err1 *protocol.Error
		var err2 *errors.Error
		switch {
		case errors.As(failure, &err1):
			status.Code = err1.Code.GetEnumValue()
		case errors.As(failure, &err2):
			status.Error = err2
			status.Code = protocol.ConvertErrorStatus(err2.Code).GetEnumValue()
		default:
			status.Code = protocol.ErrorCodeUnknownError.GetEnumValue()
		}
	})
	if err != nil {
		return nil, nil, err
	}

	// If this transaction is a synthetic transaction, send a refund
	state := new(chain.ProcessTransactionState)
	if swo, ok := transaction.Body.(protocol.SynthTxnWithOrigin); ok {
		init, refundAmount := swo.GetRefund()
		if refundAmount > 0 {
			refund := new(protocol.SyntheticDepositCredits)
			refund.Amount = refundAmount.AsUInt64()
			state.DidProduceTxn(init, refund)
		}
	}

	// Execute the post-failure hook if the transaction executor defines one
	if val, ok := getValidator[chain.TransactionExecutorCleanup](x, transaction.Body.Type()); ok {
		err = val.DidFail(state, transaction)
		if err != nil {
			return nil, nil, err
		}
	}

	// Remove the transaction from the principal's list of pending transactions
	err = batch.Account(transaction.Header.Principal).RemovePending(*(*[32]byte)(transaction.GetHash()))
	if err != nil {
		return nil, nil, fmt.Errorf("update pending list: %w", err)
	}

	// Refund the signer
	if status.Initiator == nil || !transaction.Body.Type().IsUser() {
		return status, state, nil
	}

	// TODO Send a refund for a failed remotely initiated transaction
	if !transaction.Header.Principal.LocalTo(status.Initiator) {
		return status, state, nil
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := protocol.ComputeTransactionFee(transaction)
	if err != nil {
		return nil, nil, fmt.Errorf("compute fee: %w", err)
	}
	if paid <= protocol.FeeFailedMaximum {
		return status, state, nil
	}

	var signer protocol.Signer
	obj := batch.Account(status.Initiator)
	err = obj.GetStateAs(&signer)
	if err != nil {
		return nil, nil, fmt.Errorf("load initial signer: %w", err)
	}

	refund := paid - protocol.FeeFailedMaximum
	signer.CreditCredits(refund.AsUInt64())
	err = obj.PutState(signer)
	if err != nil {
		return nil, nil, fmt.Errorf("store initial signer: %w", err)
	}

	return status, state, nil
}
