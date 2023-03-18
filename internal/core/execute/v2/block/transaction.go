// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ProcessTransaction processes a transaction. It will not return an error if
// the transaction fails - in that case the status code will be non zero. It
// only returns an error in cases like a database failure.
func (x *Executor) ProcessTransaction(batch *database.Batch, delivery *chain.Delivery) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	r := x.BlockTimers.Start(BlockTimerTypeProcessTransaction)
	defer x.BlockTimers.Stop(r)

	// Load the status
	status, err := batch.Transaction(delivery.Transaction.GetHash()).GetStatus()
	if err != nil {
		return nil, nil, err
	}

	// The status txid should not be nil, but fix it if it is *shrug*
	if status.TxID == nil && delivery.Transaction.Header.Principal != nil {
		status.TxID = delivery.Transaction.ID()
		err = batch.Transaction(delivery.Transaction.GetHash()).PutStatus(status)
		if err != nil {
			return nil, nil, err
		}
	}

	// Load the principal
	principal, err := batch.Account(delivery.Transaction.Header.Principal).GetState()
	switch {
	case err == nil, errors.Is(err, storage.ErrNotFound):
		// Ok
	default:
		err = errors.UnknownError.WithFormat("load principal: %w", err)
		return x.recordFailedTransaction(batch, delivery, err)
	}

	// Check if the transaction is ready to be executed
	ready, err := x.TransactionIsReady(batch, delivery, status, principal)
	if err != nil {
		if errors.Is(err, errors.Delivered) {
			// If a synthetic transaction is re-delivered, don't record anything
			return status, new(chain.ProcessTransactionState), nil
		}
		return x.recordFailedTransaction(batch, delivery, err)
	}
	if !ready {
		return x.recordPendingTransaction(&x.Describe, batch, delivery)
	}

	// Set up the state manager
	st := chain.NewStateManager(&x.Describe, &x.globals.Active, x, batch.Begin(true), principal, delivery.Transaction, x.logger.With("operation", "ProcessTransaction"))
	defer st.Discard()

	// Execute the transaction
	executor, ok := x.executors[delivery.Transaction.Body.Type()]
	if !ok {
		// An invalid transaction should not make it to this point
		err = errors.InternalError.WithFormat("missing executor for %v", delivery.Transaction.Body.Type())
		return x.recordFailedTransaction(batch, delivery, err)
	}

	r2 := x.BlockTimers.Start(executor.Type())
	result, err := executor.Execute(st, &chain.Delivery{Transaction: delivery.Transaction})
	x.BlockTimers.Stop(r2)
	if err != nil {
		err = errors.UnknownError.Wrap(err)
		return x.recordFailedTransaction(batch, delivery, err)
	}

	// Do extra processing for special network accounts
	err = x.processNetworkAccountUpdates(st.GetBatch(), delivery, principal)
	if err != nil {
		return x.recordFailedTransaction(batch, delivery, err)
	}

	// Commit changes, queue state creates for synthetic transactions
	state, err := st.Commit()
	if err != nil {
		err = fmt.Errorf("commit: %w", err)
		return x.recordFailedTransaction(batch, delivery, err)
	}

	return x.recordSuccessfulTransaction(batch, state, delivery, result)
}

func (x *Executor) TransactionIsReady(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, principal protocol.Account) (bool, error) {
	var ready bool
	var err error
	typ := delivery.Transaction.Body.Type()
	switch {
	case typ.IsUser():
		ready, err = x.userTransactionIsReady(batch, delivery, status, principal)
	case typ.IsSynthetic():
		ready, err = x.synthTransactionIsReady(batch, delivery, status, principal)
	case typ.IsSystem():
		ready, err = x.systemTransactionIsReady(batch, delivery, status, principal)
	default:
		return false, errors.InternalError.WithFormat("unknown transaction type %v", typ)
	}
	return ready, errors.UnknownError.Wrap(err)
}

func (x *Executor) userTransactionIsReady(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, principal protocol.Account) (bool, error) {
	isInit, _, err := x.TransactionIsInitiated(batch, delivery.Transaction)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	if !isInit {
		return false, nil
	}

	// If the principal is missing, check if that's ok
	if principal == nil {
		val, ok := getValidator[chain.PrincipalValidator](x, delivery.Transaction.Body.Type())
		if !ok || !val.AllowMissingPrincipal(delivery.Transaction) {
			return false, errors.NotFound.WithFormat("missing principal: %v not found", delivery.Transaction.Header.Principal)
		}
	}

	// Internally produced transactions are always executed immediately
	if delivery.WasProducedInternally() {
		return true, nil
	}

	// Delegate to the transaction executor?
	val, ok := getValidator[chain.SignerValidator](x, delivery.Transaction.Body.Type())
	if ok {
		ready, fallback, err := val.TransactionIsReady(x, batch, delivery.Transaction, status)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if !fallback {
			return ready, nil
		}
	}

	// At this point we cannot continue without the principal
	if principal == nil {
		return false, errors.NotFound.WithFormat("missing principal: %v not found", delivery.Transaction.Header.Principal)
	}

	// Get the principal's account auth
	auth, err := x.GetAccountAuthoritySet(batch, principal)
	if err != nil {
		return false, fmt.Errorf("unable to load authority of %v: %w", delivery.Transaction.Header.Principal, err)
	}

	// For each authority
	var authorized bool
	authRequired := delivery.Transaction.Body.Type().RequireAuthorization()
	for _, entry := range auth.Authorities {
		// Do not check signers for disabled authorities
		if entry.Disabled && !authRequired {
			continue
		}

		// Check if any signer has reached its threshold
		ok, err := x.AuthorityIsSatisfied(batch, delivery.Transaction, status, entry.Url)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if !ok {
			return false, nil
		}
		authorized = true
	}
	if authorized {
		return true, nil
	}

	// If every authority is disabled, at least one signature is required
	voters, err := batch.Account(delivery.Transaction.Header.Principal).
		Transaction(delivery.Transaction.ID().Hash()).
		Votes().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load voters: %w", err)
	}

	return len(voters) > 0, nil
}

func (x *Executor) AuthorityIsSatisfied(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, authUrl *url.URL) (bool, error) {
	_, err := batch.
		Account(transaction.Header.Principal).
		Transaction(transaction.ID().Hash()).
		Votes().Find(&database.VoteEntry{Authority: authUrl})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errors.NotFound):
		return false, nil
	default:
		return false, errors.UnknownError.With("load vote: %w", err)
	}
}

func (x *Executor) AuthorityIsReady(batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, authUrl *url.URL) (bool, error) {
	for _, signer := range status.FindSigners(authUrl) {
		ok, err := x.SignerIsSatisfied(batch, transaction, status, signer)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
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

	// Check if the threshold has been reached
	if uint64(signatures.Count()) >= signer.GetSignatureThreshold() {
		return true, nil
	}

	return false, nil
}

func (x *Executor) synthTransactionIsReady(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, principal protocol.Account) (bool, error) {
	// Do not check the principal until the transaction is ready (see below). Do
	// not delegate "is ready?" to the transaction executor - synthetic
	// transactions _must_ be sequenced and proven before being executed.

	// Sequence checking code has been moved to the SequencedMessage executor

	if principal != nil {
		return true, nil
	}

	// If the principal is required but missing, do not return an error unless
	// the transaction is ready to execute.
	// https://accumulate.atlassian.net/browse/AC-1704
	val, ok := getValidator[chain.PrincipalValidator](x, delivery.Transaction.Body.Type())
	if !ok || !val.AllowMissingPrincipal(delivery.Transaction) {
		return false, errors.NotFound.WithFormat("missing principal: %v not found", delivery.Transaction.Header.Principal)
	}

	return true, nil
}

func (x *Executor) systemTransactionIsReady(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, principal protocol.Account) (bool, error) {
	// Do not check the principal until the transaction is ready (see below). Do
	// not delegate "is ready?" to the transaction executor - anchors _must_ be
	// sequenced.

	switch delivery.Transaction.Body.Type() {
	case protocol.TransactionTypeSystemGenesis, protocol.TransactionTypeSystemWriteData:
		// Do not check these
		return true, nil

	default:
		// Anchors must be sequenced
	}

	// Anchor signature checking code has been moved to the BlockAnchor executor

	// Sequence checking code has been moved to the SequencedMessage executor

	if principal != nil {
		return true, nil
	}

	// If the principal is required but missing, do not return an error unless
	// the transaction is ready to execute.
	// https://accumulate.atlassian.net/browse/AC-1704
	val, ok := getValidator[chain.PrincipalValidator](x, delivery.Transaction.Body.Type())
	if !ok || !val.AllowMissingPrincipal(delivery.Transaction) {
		return false, errors.NotFound.WithFormat("missing principal: %v not found", delivery.Transaction.Header.Principal)
	}

	return true, nil
}

func (x *Executor) recordTransaction(batch *database.Batch, delivery *chain.Delivery, state *chain.ProcessTransactionState, updateStatus func(*protocol.TransactionStatus)) (*protocol.TransactionStatus, error) {
	// Store the transaction state (without signatures)
	//
	// TODO This should not always be a UserTransaction
	err := batch.Message(delivery.Transaction.ID().Hash()).Main().Put(&messaging.TransactionMessage{Transaction: delivery.Transaction})
	if err != nil {
		return nil, fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	db := batch.Transaction(delivery.Transaction.GetHash())
	status, err := db.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction status: %w", err)
	}

	status.TxID = delivery.Transaction.ID()
	updateStatus(status)
	err = db.PutStatus(status)
	if err != nil {
		return nil, fmt.Errorf("store transaction status: %w", err)
	}

	return status, nil
}

func (x *Executor) recordPendingTransaction(net *config.Describe, batch *database.Batch, delivery *chain.Delivery) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	state := new(chain.ProcessTransactionState)
	status, err := x.recordTransaction(batch, delivery, state, func(status *protocol.TransactionStatus) {
		status.Code = errors.Pending
	})
	if err != nil {
		return nil, nil, err
	}

	if delivery.Transaction.Body.Type().IsSystem() {
		return status, state, nil
	}

	if delivery.Transaction.Body.Type().IsSynthetic() {
		x.logger.Debug("Pending synthetic transaction", "hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4), "type", delivery.Transaction.Body.Type(), "module", "synthetic")
		return status, state, nil
	}

	// Add the user transaction to the principal's list of pending transactions
	err = batch.Account(delivery.Transaction.Header.Principal).AddPending(delivery.Transaction.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("store pending list: %w", err)
	}

	return status, state, nil
}

func (x *Executor) recordSuccessfulTransaction(batch *database.Batch, state *chain.ProcessTransactionState, delivery *chain.Delivery, result protocol.TransactionResult) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	status, err := x.recordTransaction(batch, delivery, state, func(status *protocol.TransactionStatus) {
		status.Code = errors.Delivered
		if result == nil {
			status.Result = new(protocol.EmptyResult)
		} else {
			status.Result = result
		}
	})
	if err != nil {
		return nil, nil, err
	}

	// Don't do anything else for Genesis or SystemWriteData
	typ := delivery.Transaction.Body.Type()
	if typ.IsSystem() && !typ.IsAnchor() {
		return status, state, nil
	}

	// Remove the transaction from the principal's list of pending transactions
	record := batch.Account(delivery.Transaction.Header.Principal)
	err = record.RemovePending(delivery.Transaction.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("store pending list: %w", err)
	}

	// Add the transaction to the principal's main or scratch chain
	chain := selectTargetChain(record, delivery.Transaction.Body)
	err = state.ChainUpdates.AddChainEntry(batch, chain, delivery.Transaction.GetHash(), 0, 0)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, fmt.Errorf("add to chain: %v", err)
	}

	return status, state, nil
}

func selectTargetChain(account *database.Account, body protocol.TransactionBody) *database.Chain2 {
	switch body := body.(type) {
	case *protocol.WriteData:
		if body.Scratch {
			return account.ScratchChain()
		}
	case *protocol.TransferCredits:
		return account.ScratchChain()
	}
	return account.MainChain()
}

func (x *Executor) recordFailedTransaction(batch *database.Batch, delivery *chain.Delivery, failure error) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Record the transaction
	state := new(chain.ProcessTransactionState)
	status, err := x.recordTransaction(batch, delivery, state, func(status *protocol.TransactionStatus) {
		status.Set(failure)
	})
	if err != nil {
		return nil, nil, err
	}

	// If this transaction is a synthetic transaction, send a refund
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

	// Issue a refund to the initial signer
	isInit, initiator, err := x.TransactionIsInitiated(batch, delivery.Transaction)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	if !isInit || !delivery.Transaction.Body.Type().IsUser() {
		return status, state, nil
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := x.globals.Active.Globals.FeeSchedule.ComputeTransactionFee(delivery.Transaction)
	if err != nil {
		return nil, nil, fmt.Errorf("compute fee: %w", err)
	}
	if paid <= protocol.FeeFailedMaximum {
		return status, state, nil
	}

	refund := new(protocol.SyntheticDepositCredits)
	refund.Amount = (paid - protocol.FeeFailedMaximum).AsUInt64()
	state.DidProduceTxn(initiator.Payer, refund)
	return status, state, nil
}
