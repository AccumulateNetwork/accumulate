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
func (x *Executor) ProcessTransaction(batch *database.Batch, delivery *chain.Delivery) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Load the status
	status, err := batch.Transaction(delivery.Transaction.GetHash()).GetStatus()
	if err != nil {
		return nil, nil, err
	}
	if status.Initiator == nil {
		// This should never happen
		return nil, nil, fmt.Errorf("transaction initiator is missing")
	}

	// Workaround for AC-1704 - do an extra check to prevent out-of-sequence
	// synthetic transactions from failing when loading the principal
	if !delivery.WasProducedInternally() && delivery.Transaction.Body.Type().IsSynthetic() {
		ready, err := x.synthTransactionIsReady(batch, delivery.Transaction, status)
		if err != nil {
			return x.recordFailedTransaction(batch, delivery, err)
		}
		if !ready {
			return x.recordPendingTransaction(&x.Network, batch, delivery)
		}
	}

	// Load the principal
	principal, err := batch.Account(delivery.Transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, storage.ErrNotFound):
		err = errors.Format(errors.StatusUnknown, "load principal: %w", err)
		return x.recordFailedTransaction(batch, delivery, err)
	case !x.transactionAllowsMissingPrincipal(delivery.Transaction):
		err = errors.Format(errors.StatusUnknown, "load principal: %w", err)
		return x.recordFailedTransaction(batch, delivery, err)
	}

	if !delivery.WasProducedInternally() {
		// Check if the transaction is ready to be executed
		ready, err := x.TransactionIsReady(batch, delivery.Transaction, status)
		if err != nil {
			return x.recordFailedTransaction(batch, delivery, err)
		}
		if !ready {
			return x.recordPendingTransaction(&x.Network, batch, delivery)
		}
	}

	if delivery.Transaction.Body.Type().IsSynthetic() {
		// Verify that the synthetic transaction has all the right signatures
		err = processSyntheticTransaction(batch, delivery.Transaction, status)
		if err != nil {
			return x.recordFailedTransaction(batch, delivery, err)
		}
	}

	// Set up the state manager
	var st *chain.StateManager
	if x.isGenesis {
		st = chain.NewStateManager(&x.Network, nil, batch.Begin(true), principal, delivery.Transaction, x.logger.With("operation", "ProcessTransaction"))
	} else {
		st, err = chain.LoadStateManager(&x.Network, &x.globals.Active, batch.Begin(true), principal, delivery.Transaction, status, x.logger.With("operation", "ProcessTransaction"))
		if err != nil {
			return x.recordFailedTransaction(batch, delivery, err)
		}
	}
	defer st.Discard()

	// Execute the transaction
	executor, ok := x.executors[delivery.Transaction.Body.Type()]
	if !ok {
		// An invalid transaction should not make it to this point
		err = protocol.Errorf(protocol.ErrorCodeInternal, "missing executor for %v", delivery.Transaction.Body.Type())
		return x.recordFailedTransaction(batch, delivery, err)
	}

	result, err := executor.Execute(st, &chain.Delivery{Transaction: delivery.Transaction})
	if err != nil {
		err = errors.Wrap(0, err)
		return x.recordFailedTransaction(batch, delivery, err)
	}

	// Commit changes, queue state creates for synthetic transactions
	state, err := st.Commit()
	if err != nil {
		err = fmt.Errorf("commit: %w", err)
		return x.recordFailedTransaction(batch, delivery, err)
	}

	// Do extra processing for special network accounts
	if principal != nil && principal.GetUrl().RootIdentity().Equal(x.Network.NodeUrl()) {
		err = x.processNetworkAccountUpdates(batch, delivery, principal)
		if err != nil {
			return x.recordFailedTransaction(batch, delivery, err)
		}

		// Only push sync updates for DN accounts
		if x.Network.Type == config.Directory {
			err = x.pushNetworkAccountUpdates(batch, delivery, principal)
			if err != nil {
				return x.recordFailedTransaction(batch, delivery, err)
			}
		}
	}

	return x.recordSuccessfulTransaction(batch, state, delivery, result)
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
	authRequired := transaction.Body.Type().RequireAuthorization()
	for _, entry := range auth.Authorities {
		// Do not check signers for disabled authorities
		if entry.Disabled && !authRequired {
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
	anchorChain, err := batch.Account(x.Network.AnchorPool()).ReadChain(protocol.RootAnchorChain(subnet))
	if err != nil {
		return false, errors.Format(errors.StatusUnknown, "load %s intermediate anchor chain: %w", subnet, err)
	}

	// Is the result a valid DN anchor?
	_, err = anchorChain.HeightOf(receipt.Anchor)
	switch {
	case err == nil:
		// Ready
	case errors.Is(err, storage.ErrNotFound):
		return false, nil
	default:
		return false, errors.Format(errors.StatusUnknown, "get height of entry %X of %s intermediate anchor chain: %w", receipt.Anchor[:4], subnet, err)
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

	// If the transaction is out of sequence, mark it pending
	subnetLedger := ledger.Subnet(synthSig.SourceNetwork)
	if subnetLedger.Delivered+1 != synthSig.SequenceNumber {
		x.logger.Info("Out of sequence synthetic transaction",
			"hash", logging.AsHex(transaction.GetHash()).Slice(0, 4),
			"seq-got", synthSig.SequenceNumber,
			"seq-want", subnetLedger.Delivered+1,
			"source", synthSig.SourceNetwork,
			"destination", synthSig.DestinationNetwork,
			"type", transaction.Body.Type(),
			"hash", logging.AsHex(transaction.GetHash()).Slice(0, 4),
		)
		return false, nil
	}

	return true, nil
}

func (x *Executor) recordTransaction(batch *database.Batch, delivery *chain.Delivery, updateStatus func(*protocol.TransactionStatus)) (*protocol.TransactionStatus, error) {
	// Store the transaction state (without signatures)
	db := batch.Transaction(delivery.Transaction.GetHash())
	err := db.PutState(&database.SigOrTxn{Transaction: delivery.Transaction})
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

	// If the transaction is synthetic, update the synthetic ledger
	if !delivery.Transaction.Body.Type().IsSynthetic() {
		return status, nil
	}

	// Update the synthetic ledger
	var ledger *protocol.SyntheticLedger
	err = batch.Account(x.Network.Synthetic()).GetStateAs(&ledger)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load synthetic transaction ledger: %w", err)
	}

	subnetLedger := ledger.Subnet(delivery.SourceNetwork)
	if delivery.SequenceNumber == subnetLedger.Delivered {
		x.logger.Error("Synthetic transaction sequence number has already been delivered", "sequence", delivery.SequenceNumber, "hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4), "type", delivery.Transaction.Body.Type())
	} else if delivery.SequenceNumber < subnetLedger.Delivered {
		x.logger.Error("A synthetic transaction was delivered out of order", "received-seq", delivery.SequenceNumber, "delivered-seq", subnetLedger.Delivered, "received-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4), "received-type", delivery.Transaction.Body.Type())
	} else if subnetLedger.Add(status.Delivered, delivery.SequenceNumber, delivery.Transaction.ID()) {
		err = batch.Account(x.Network.Synthetic()).PutState(ledger)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknown, "store synthetic transaction ledger: %w", err)
		}
	}

	return status, nil
}

func (x *Executor) recordPendingTransaction(net *config.Network, batch *database.Batch, delivery *chain.Delivery) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := x.recordTransaction(batch, delivery, func(status *protocol.TransactionStatus) {
		status.Remote = false
		status.Pending = true
	})
	if err != nil {
		return nil, nil, err
	}

	// Add the user transaction to the principal's list of pending transactions
	if delivery.Transaction.Body.Type().IsUser() {
		err = batch.Account(delivery.Transaction.Header.Principal).AddPending(delivery.Transaction.ID())
		if err != nil {
			return nil, nil, fmt.Errorf("store pending list: %w", err)
		}

		return status, new(chain.ProcessTransactionState), nil
	}

	// Load all of the signatures
	signatures, err := GetAllSignatures(batch, batch.Transaction(delivery.Transaction.GetHash()), status, delivery.Transaction.Header.Initiator[:])
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Add the synthetic transaction to the anchor's list of pending transactions
	receipt, _, err := assembleSynthReceipt(delivery.Transaction, signatures)
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	if receipt == nil {
		x.logger.Error("Missing receipt for pending synthetic transaction", "hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4), "type", delivery.Transaction.Body.Type())
		return status, new(chain.ProcessTransactionState), nil
	}

	err = batch.Account(net.Ledger()).AddSyntheticForAnchor(*(*[32]byte)(receipt.Anchor), delivery.Transaction.ID())
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)
	}

	return status, new(chain.ProcessTransactionState), nil
}

func (x *Executor) recordSuccessfulTransaction(batch *database.Batch, state *chain.ProcessTransactionState, delivery *chain.Delivery, result protocol.TransactionResult) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := x.recordTransaction(batch, delivery, func(status *protocol.TransactionStatus) {
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
	if delivery.Transaction.Body.Type().IsSystem() {
		return status, state, nil
	}

	// Remove the transaction from the principal's list of pending transactions
	err = batch.Account(delivery.Transaction.Header.Principal).RemovePending(delivery.Transaction.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("store pending list: %w", err)
	}

	// Add the transaction to the principal's main chain
	err = state.ChainUpdates.AddChainEntry(batch, delivery.Transaction.Header.Principal, protocol.MainChain, protocol.ChainTypeTransaction, delivery.Transaction.GetHash(), 0, 0)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, fmt.Errorf("add to chain: %v", err)
	}

	if !delivery.Transaction.Body.Type().IsSynthetic() /*|| delivery.Transaction.Body.Type() == protocol.TransactionTypeSegWitDataEntry*/ {
		return status, state, nil
	}

	// Check for pending synthetic transactions
	var ledger *protocol.SyntheticLedger
	err = batch.Account(x.Network.Synthetic()).GetStateAs(&ledger)
	if err != nil {
		return nil, nil, errors.Format(errors.StatusUnknown, "load synthetic transaction ledger: %w", err)
	}

	nextHash, ok := ledger.Subnet(delivery.SourceNetwork).Get(delivery.SequenceNumber + 1)
	if ok {
		state.ProcessAdditionalTransaction(delivery.NewSyntheticFromSequence(nextHash.Hash()))
	}

	return status, state, nil
}

func (x *Executor) recordFailedTransaction(batch *database.Batch, delivery *chain.Delivery, failure error) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := x.recordTransaction(batch, delivery, func(status *protocol.TransactionStatus) {
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
	if swo, ok := delivery.Transaction.Body.(protocol.SynthTxnWithOrigin); ok {
		init, refundAmount := swo.GetRefund()
		if refundAmount > 0 {
			refund := new(protocol.SyntheticDepositCredits)
			refund.Amount = refundAmount.AsUInt64()
			state.DidProduceTxn(init, refund)
		}
	}

	// Execute the post-failure hook if the transaction executor defines one
	if val, ok := getValidator[chain.TransactionExecutorCleanup](x, delivery.Transaction.Body.Type()); ok {
		err = val.DidFail(state, delivery.Transaction)
		if err != nil {
			return nil, nil, err
		}
	}

	// Remove the transaction from the principal's list of pending transactions
	err = batch.Account(delivery.Transaction.Header.Principal).RemovePending(delivery.Transaction.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("update pending list: %w", err)
	}

	// Refund the signer
	if status.Initiator == nil || !delivery.Transaction.Body.Type().IsUser() {
		return status, state, nil
	}

	// TODO Send a refund for a failed remotely initiated transaction
	if !delivery.Transaction.Header.Principal.LocalTo(status.Initiator) {
		return status, state, nil
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := protocol.ComputeTransactionFee(delivery.Transaction)
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

// processNetworkAccountUpdates processes updates to network data accounts,
// updating the in-memory globals variable.
func (x *Executor) processNetworkAccountUpdates(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) error {
	// Only WriteData needs extra processing
	body, ok := delivery.Transaction.Body.(*protocol.WriteData)
	if !ok {
		return nil
	}

	// Force WriteToState
	if !body.WriteToState {
		return errors.Format(errors.StatusBadRequest, "invalid %v update: network account updates must write to state", principal.GetUrl())
	}

	// Validate the data and update the corresponding variable
	switch {
	case principal.GetUrl().PathEqual(protocol.Oracle):
		return x.globals.Pending.ParseOracle(body.Entry)
	case principal.GetUrl().PathEqual(protocol.Globals):
		return x.globals.Pending.ParseGlobals(body.Entry)
	case principal.GetUrl().PathEqual(protocol.Network):
		return x.globals.Pending.ParseNetwork(body.Entry)
	}

	return nil
}

// pushNetworkAccountUpdates pushes updates from the DN to the BVNs.
func (x *Executor) pushNetworkAccountUpdates(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) error {
	// Prep the update
	var update protocol.NetworkAccountUpdate
	update.Body = delivery.Transaction.Body

	switch delivery.Transaction.Body.Type() {
	case protocol.TransactionTypeUpdateKeyPage:
		// Synchronize updates to the operator book
		if u, ok := principal.GetUrl().Parent(); ok && u.PathEqual(protocol.OperatorBook) {
			update.Name = protocol.OperatorBook
			break
		}

	case protocol.TransactionTypeWriteData:
		// Synchronize updates to the oracle, globals, and network definition
		switch {
		case principal.GetUrl().PathEqual(protocol.Oracle):
			update.Name = protocol.Oracle
		case principal.GetUrl().PathEqual(protocol.Globals):
			update.Name = protocol.Globals
		case principal.GetUrl().PathEqual(protocol.Network):
			update.Name = protocol.Network
		}
	}

	// No update needed
	if update.Name == "" {
		return nil
	}

	// Write the update to the ledger
	var ledger *protocol.SystemLedger
	record := batch.Account(x.Network.Ledger())
	err := record.GetStateAs(&ledger)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load ledger: %w", err)
	}

	ledger.PendingUpdates = append(ledger.PendingUpdates, update)
	err = record.PutState(ledger)
	if err != nil {
		return errors.Format(errors.StatusUnknown, "store ledger: %w", err)
	}

	return nil
}
