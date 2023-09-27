// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// processTransaction processes a transaction. It will not return an error if
// the transaction fails - in that case the status code will be non zero. It
// only returns an error in cases like a database failure.
func (t *TransactionContext) processTransaction(batch *database.Batch) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	x := t.Executor
	delivery := &chain.Delivery{
		Transaction: t.transaction,
		Internal:    t.isWithin(internal.MessageTypeNetworkUpdate),
	}

	r := x.BlockTimers.Start(BlockTimerTypeProcessTransaction)
	defer x.BlockTimers.Stop(r)

	// Load the status
	status, err := batch.Transaction(delivery.Transaction.GetHash()).Status().Get()
	if err != nil {
		return nil, nil, err
	}

	// The status txid should not be nil, but fix it if it is *shrug*
	if status.TxID == nil && delivery.Transaction.Header.Principal != nil {
		status.TxID = delivery.Transaction.ID()
		err = batch.Transaction(delivery.Transaction.GetHash()).Status().Put(status)
		if err != nil {
			return nil, nil, err
		}
	}

	// Load the principal
	principal, err := batch.Account(delivery.Transaction.Header.Principal).Main().Get()
	switch {
	case err == nil, errors.Is(err, storage.ErrNotFound):
		// Ok
	default:
		err = errors.UnknownError.WithFormat("load principal: %w", err)
		return t.recordFailedTransaction(batch, delivery, err)
	}

	// Check if the transaction is ready to be executed
	ready, err := t.transactionIsReady(batch, delivery, status, principal)
	if err != nil {
		if errors.Is(err, errors.Delivered) {
			// If a synthetic transaction is re-delivered, don't record anything
			return status, new(chain.ProcessTransactionState), nil
		}
		return t.recordFailedTransaction(batch, delivery, err)
	}
	if !ready {
		return t.recordPendingTransaction(x.Describe, batch, delivery)
	}

	// Set up the state manager
	st := chain.NewStateManager(x.Describe, &x.globals.Active, t, batch.Begin(true), principal, delivery.Transaction, x.logger.With("operation", "ProcessTransaction"))
	defer st.Discard()

	// Execute the transaction
	executor, ok := x.executors[delivery.Transaction.Body.Type()]
	if !ok {
		// An invalid transaction should not make it to this point
		err = errors.InternalError.WithFormat("missing executor for %v", delivery.Transaction.Body.Type())
		return t.recordFailedTransaction(batch, delivery, err)
	}

	r2 := x.BlockTimers.Start(executor.Type())
	result, err := executor.Execute(st, &chain.Delivery{Transaction: delivery.Transaction})
	x.BlockTimers.Stop(r2)
	if err != nil {
		err = errors.UnknownError.Wrap(err)
		return t.recordFailedTransaction(batch, delivery, err)
	}

	// Do extra processing for special network accounts
	err = x.processNetworkAccountUpdates(st.GetBatch(), delivery, principal)
	if err != nil {
		return t.recordFailedTransaction(batch, delivery, err)
	}

	// Commit changes, queue state creates for synthetic transactions
	state, err := st.Commit()
	if err != nil {
		err = fmt.Errorf("commit: %w", err)
		return t.recordFailedTransaction(batch, delivery, err)
	}

	return t.recordSuccessfulTransaction(batch, state, delivery, result)
}

func (t *TransactionContext) transactionIsReady(batch *database.Batch, delivery *chain.Delivery, status *protocol.TransactionStatus, principal protocol.Account) (bool, error) {
	var ready bool
	var err error
	typ := delivery.Transaction.Body.Type()
	switch {
	case typ.IsUser():
		ready, err = t.userTransactionIsReady(batch, delivery, principal)
	case typ.IsSynthetic():
		ready, err = t.Executor.synthTransactionIsReady(batch, delivery, principal)
	case typ.IsSystem():
		ready, err = t.Executor.systemTransactionIsReady(batch, delivery, principal)
	default:
		return false, errors.InternalError.WithFormat("unknown transaction type %v", typ)
	}
	return ready, errors.UnknownError.Wrap(err)
}

func (t *TransactionContext) userTransactionIsReady(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) (bool, error) {
	x := t.Executor
	isInit, _, err := transactionIsInitiated(batch, delivery.Transaction.ID())
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
		ready, fallback, err := val.TransactionIsReady(t, batch, delivery.Transaction)
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
	auth, err := getAccountAuthoritySet(batch, principal)
	if err != nil {
		return false, fmt.Errorf("unable to load authority of %v: %w", delivery.Transaction.Header.Principal, err)
	}

	// At a minimum (if every authority is disabled), at least one signature is
	// required
	voters, err := batch.Account(delivery.Transaction.Header.Principal).
		Transaction(delivery.Transaction.ID().Hash()).
		Votes().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load voters: %w", err)
	}
	if len(voters) == 0 {
		return false, nil
	}

	// For each authority
	notReady := map[[32]byte]struct{}{}
	ignoreDisabled := delivery.Transaction.Body.Type().RequireAuthorization()
	for _, entry := range auth.Authorities {
		// Ignore disabled authorities
		if entry.Disabled && !ignoreDisabled {
			continue
		}

		// Check if any signer has reached its threshold
		ok, vote, err := t.AuthorityDidVote(batch, delivery.Transaction, entry.Url)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}

		// Did the authority vote?
		if !ok {
			notReady[entry.Url.AccountID32()] = struct{}{}
			continue
		}

		// Any vote that is not accept is counted as reject. Since a transaction
		// is only executed once _all_ authorities accept, a single authority
		// rejecting or abstaining is sufficient to block the transaction.
		if vote != protocol.VoteTypeAccept {
			return false, errors.Rejected
		}
	}

	// The transaction is only ready if all authorities have voted
	return len(notReady) == 0, nil
}

func (m *MessageContext) AuthorityDidVote(batch *database.Batch, transaction *protocol.Transaction, authUrl *url.URL) (bool, protocol.VoteType, error) {
	// Find the vote
	entry, err := batch.
		Account(transaction.Header.Principal).
		Transaction(transaction.ID().Hash()).
		Votes().Find(&database.VoteEntry{Authority: authUrl})
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return false, 0, nil
	default:
		return false, 0, errors.UnknownError.With("load vote entry: %w", err)
	}

	// Load the vote
	sig, err := m.GetSignatureAs(batch, entry.Hash)
	if err != nil {
		return false, 0, errors.UnknownError.WithFormat("load vote %v: %w", transaction.Header.Principal.WithTxID(entry.Hash), err)
	}

	// Verify it is an authority signature
	auth, ok := sig.(*protocol.AuthoritySignature)
	if !ok {
		return false, 0, errors.InternalError.WithFormat("invalid vote: expected %v, got %v", protocol.SignatureTypeAuthority, sig.Type())
	}
	return true, auth.Vote, nil
}

// AuthorityWillVote checks if the authority is ready to vote. MUST NOT MODIFY
// STATE.
func (m *MessageContext) AuthorityWillVote(batch *database.Batch, block uint64, transaction *protocol.Transaction, authUrl *url.URL) (*chain.AuthVote, error) {
	var authority protocol.Authority
	err := batch.Account(authUrl).Main().GetAs(&authority)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load authority %v: %w", authUrl, err)
	}

	for _, signer := range authority.GetSigners() {
		ok, vote, err := m.SignerWillVote(batch, block, transaction, signer)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if ok {
			return &chain.AuthVote{Source: signer, Vote: vote}, nil
		}
	}

	return nil, nil
}

// SignerWillVote checks if the signer is ready to vote. MUST NOT MODIFY STATE.
func (m *MessageContext) SignerWillVote(batch *database.Batch, block uint64, transaction *protocol.Transaction, signerUrl *url.URL) (bool, protocol.VoteType, error) {
	// Load the signer
	var signer protocol.Signer
	err := batch.Account(signerUrl).Main().GetAs(&signer)
	if err != nil {
		return false, 0, errors.UnknownError.WithFormat("load signer %v: %w", signerUrl, err)
	}

	// Get thresholds
	var tReject, tResponse uint64
	tAccept := signer.GetSignatureThreshold()
	entryCount := 1

	if page, ok := signer.(*protocol.KeyPage); ok {
		tReject = page.RejectThreshold
		tResponse = page.ResponseThreshold
		entryCount = len(page.Keys)
	}

	if tReject == 0 {
		tReject = tAccept
	}

	// Load the active signature set
	entries, err := batch.
		Account(signerUrl).
		Transaction(transaction.ID().Hash()).
		Signatures().
		Get()
	if err != nil {
		return false, 0, errors.UnknownError.WithFormat("load %v signatures: %w", signerUrl, err)
	}

	// Add up the votes for each delegation path
	pathVoteCount := map[[32]byte]map[protocol.VoteType]uint64{}
	for _, entry := range entries {
		sig, err := m.GetSignatureAs(batch, entry.Hash)
		if err != nil {
			return false, 0, errors.UnknownError.WithFormat("load %v: %w", signerUrl.WithTxID(entry.Hash), err)
		}

		h := entry.PathHash()
		m, ok := pathVoteCount[h]
		if !ok {
			m = map[protocol.VoteType]uint64{}
			pathVoteCount[h] = m
		}
		m[sig.GetVote()]++
	}

	// Check each path
	for _, votes := range pathVoteCount {
		// Calculate the allVotes number of responses
		var allVotes uint64
		for _, count := range votes {
			allVotes += count
		}

		// Check the response threshold
		if allVotes < tResponse {
			continue
		}

		// Check the accept threshold
		if votes[protocol.VoteTypeAccept] >= tAccept {
			return true, protocol.VoteTypeAccept, nil
		}

		// Check the reject threshold
		if votes[protocol.VoteTypeReject] >= tReject {
			return true, protocol.VoteTypeReject, nil
		}

		// If it is not possible to reach consensus, the authority abstains. If
		// the vote can be swung by every remaining (undecided) voter voting
		// together to accept or reject, it is still possible to reach
		// consensus. Thus if that is not possible for accept, or for reject,
		// consensus is unreachable.
		undecided := uint64(entryCount) - allVotes
		couldAccept := votes[protocol.VoteTypeAccept]+undecided >= tAccept
		couldReject := votes[protocol.VoteTypeReject]+undecided >= tReject
		if !couldAccept && !couldReject {
			return true, protocol.VoteTypeAbstain, nil
		}
	}

	return false, 0, nil
}

func (m *MessageContext) blockThresholdIsMet(batch *database.Batch, transaction *protocol.Transaction) (bool, error) {
	// TODO If minimum thresholds are added (to pages, books, or accounts),
	// UpdateKey will need a way of overriding them.

	hold := transaction.Header.HoldUntil
	if hold == nil {
		return true, nil
	}

	// Check the hold index against the latest DN block height
	if hold.MinorBlock != 0 {
		dnHeight, err := getDnHeight(m.Executor.Describe, batch)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if hold.MinorBlock > dnHeight {
			return false, nil
		}
	}

	return true, nil
}

func getDnHeight(desc execute.DescribeShim, batch *database.Batch) (uint64, error) {
	c := batch.Account(desc.AnchorPool()).MainChain()
	head, err := c.Head().Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load anchor ledger main chain head: %w", err)
	}

	for i := head.Count - 1; i >= 0; i-- {
		entry, err := c.Entry(i)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load anchor ledger main chain entry %d (1): %w", i, err)
		}

		var msg *messaging.TransactionMessage
		err = batch.Message2(entry).Main().GetAs(&msg)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load anchor ledger main chain entry %d (2): %w", i, err)
		}

		body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
		if ok {
			return body.MinorBlockIndex, nil
		}
	}

	return 0, nil
}

func (x *Executor) synthTransactionIsReady(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) (bool, error) {
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

func (x *Executor) systemTransactionIsReady(batch *database.Batch, delivery *chain.Delivery, principal protocol.Account) (bool, error) {
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

func recordTransaction(batch *database.Batch, delivery *chain.Delivery, state *chain.ProcessTransactionState, updateStatus func(*protocol.TransactionStatus)) (*protocol.TransactionStatus, error) {
	// Store the transaction state (without signatures)
	//
	// TODO This should not always be a UserTransaction
	err := batch.Message(delivery.Transaction.ID().Hash()).Main().Put(&messaging.TransactionMessage{Transaction: delivery.Transaction})
	if err != nil {
		return nil, fmt.Errorf("store transaction: %w", err)
	}

	// Update the status
	db := batch.Transaction(delivery.Transaction.GetHash())
	status, err := db.Status().Get()
	if err != nil {
		return nil, fmt.Errorf("load transaction status: %w", err)
	}

	status.TxID = delivery.Transaction.ID()
	updateStatus(status)
	err = db.Status().Put(status)
	if err != nil {
		return nil, fmt.Errorf("store transaction status: %w", err)
	}

	return status, nil
}

func (x *TransactionContext) recordPendingTransaction(net execute.DescribeShim, batch *database.Batch, delivery *chain.Delivery) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	// Do not mark pending because we only want to do that if the transaction
	// was just initiated

	// Record the transaction
	state := new(chain.ProcessTransactionState)
	status, err := recordTransaction(batch, delivery, state, func(status *protocol.TransactionStatus) {
		status.Code = errors.Pending
	})
	if err != nil {
		return nil, nil, err
	}

	if delivery.Transaction.Body.Type().IsSystem() {
		return status, state, nil
	}

	if delivery.Transaction.Body.Type().IsSynthetic() {
		x.Executor.logger.Debug("Pending synthetic transaction", "hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4), "type", delivery.Transaction.Body.Type(), "module", "synthetic")
		return status, state, nil
	}

	// Add the user transaction to the principal's list of pending transactions
	err = batch.Account(delivery.Transaction.Header.Principal).Pending().Add(delivery.Transaction.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("store pending list: %w", err)
	}

	return status, state, nil
}

func (x *TransactionContext) recordSuccessfulTransaction(batch *database.Batch, state *chain.ProcessTransactionState, delivery *chain.Delivery, result protocol.TransactionResult) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	x.State.MarkTransactionDelivered(delivery.Transaction.ID())

	// Record the transaction
	status, err := recordTransaction(batch, delivery, state, func(status *protocol.TransactionStatus) {
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
	err = record.Pending().Remove(delivery.Transaction.ID())
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

func (x *TransactionContext) recordFailedTransaction(batch *database.Batch, delivery *chain.Delivery, failure error) (*protocol.TransactionStatus, *chain.ProcessTransactionState, error) {
	x.State.MarkTransactionDelivered(delivery.Transaction.ID())

	// Record the transaction
	state := new(chain.ProcessTransactionState)
	status, err := recordTransaction(batch, delivery, state, func(status *protocol.TransactionStatus) {
		status.Set(failure)
	})
	if err != nil {
		return nil, nil, err
	}

	// If this transaction is a synthetic transaction, send a refund
	if swo, ok := delivery.Transaction.Body.(protocol.SyntheticTransaction); ok {
		init, refundAmount := swo.GetRefund()
		if refundAmount > 0 {
			refund := new(protocol.SyntheticDepositCredits)
			refund.Amount = refundAmount.AsUInt64()
			state.DidProduceTxn(init, refund)
		}
	}

	// Execute the post-failure hook if the transaction executor defines one
	if val, ok := getValidator[chain.TransactionExecutorCleanup](x.Executor, delivery.Transaction.Body.Type()); ok {
		err = val.DidFail(state, delivery.Transaction)
		if err != nil {
			return nil, nil, err
		}
	}

	// Remove the transaction from the principal's list of pending transactions
	err = batch.Account(delivery.Transaction.Header.Principal).Pending().Remove(delivery.Transaction.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("update pending list: %w", err)
	}

	// Issue a refund to the initial signer
	isInit, initiator, err := transactionIsInitiated(batch, delivery.Transaction.ID())
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	if !isInit || !delivery.Transaction.Body.Type().IsUser() {
		return status, state, nil
	}

	// But only if the paid paid is larger than the max failure paid
	paid, err := x.Executor.globals.Active.Globals.FeeSchedule.ComputeTransactionFee(delivery.Transaction)
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
