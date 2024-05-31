// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[TransactionMessage](&messageExecutors, messaging.MessageTypeTransaction)
	registerSimpleExec[ExpiredTransaction](&messageExecutors, internal.MessageTypeExpiredTransaction)
}

// TransactionMessage records the transaction but does not execute it. Transactions
// are executed in response to _authority signature_ messages, not user
// transaction messages.
type TransactionMessage struct{}

func (x TransactionMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// If the message has already been processed, return its recorded status
	status, err := batch.Transaction2(ctx.message.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Delivered() {
		// We need to remove any logical dependence on transaction statuses, due
		// to the issues they present with pruning. For now, replace error codes
		// with Delivered.
		status.Error = nil
		status.Code = errors.Delivered
		return status, nil
	}

	// As long as the transaction is well-formed, let it into the block. There
	// are many cases where we cannot safely evaluate the transaction at this
	// point. If we are evaluating a synthetic transaction, we _must not_ reject
	// it as long as it is properly formed and has a proof, since rejecting it
	// otherwise would cause problems for sequencing. If Alice initiates a
	// transaction for Bob, Bob may not be on this partition so we cannot
	// evaluate the transaction. And even in cases where we could safely
	// evaluate the transaction, doing so would cause inconsistencies: the
	// authority a user uses to initiate a transaction and which partitions the
	// accounts are on would become a factor in whether or not a transaction
	// makes it into the block. Besides that, there's the argument FairyProof
	// made that the previous approach (rejecting the transaction due to things
	// like an insufficient balance) could be considered a replay attack vector.
	// Thus, as long as the transaction is well-formed, signed, and the signer
	// can be charged _something_, we will let the transaction into the block.
	//
	// And don't resolve remote transactions here, since that would make
	// validation dependent on what has and has not been pruned, which is a
	// dangerous game to play.
	txn, err := x.check(batch, ctx, false)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Run stateless validation checks
	exec, ok := ctx.Executor.executors[txn.Transaction.Body.Type()]
	if !ok {
		return nil, nil
	}

	st := chain.NewStatelessManager(ctx.Executor.Describe, ctx.GetActiveGlobals(), txn.Transaction, ctx.Executor.logger.With("operation", "Validate"))
	st.Pretend = true

	r, err := exec.Validate(st, &chain.Delivery{Transaction: txn.Transaction})
	if err != nil {
		if !errors.Code(err).IsKnownError() {
			// Assume errors with no code are user errors
			return nil, errors.BadRequest.Wrap(err)
		}
		return nil, errors.UnknownError.Wrap(err)
	}
	if r == nil {
		return nil, nil
	}
	s := new(protocol.TransactionStatus)
	s.TxID = txn.ID()
	s.Result = r
	return s, nil
}

func (x TransactionMessage) check(batch *database.Batch, ctx *MessageContext, resolve bool) (*messaging.TransactionMessage, error) {
	txn, ok := ctx.message.(*messaging.TransactionMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeTransaction, ctx.message.Type())
	}

	// Basic validation
	if txn.Transaction == nil {
		return nil, errors.BadRequest.With("missing transaction")
	}
	if txn.Transaction.Body == nil {
		return nil, errors.BadRequest.With("missing transaction body")
	}
	if txn.Transaction.BodyIs64Bytes() {
		return nil, errors.BadRequest.WithFormat("cannot process transaction: body is 64 bytes long")
	}

	isRemote := txn.Transaction.Body.Type() == protocol.TransactionTypeRemote
	if !isRemote {
		if txn.Transaction.Header.Principal == nil {
			return nil, errors.BadRequest.With("missing principal")
		}
		if txn.Transaction.Body.Type().IsUser() && txn.Transaction.Header.Initiator == ([32]byte{}) {
			return nil, errors.BadRequest.With("missing initiator")
		}
	}

	// Make sure user transactions are signed. Synthetic messages and network
	// update messages do not require signatures.
	//
	// If we're within MessageIsReady, presumably this has already been checked.
	// But if we're within MessageIsReady that is itself within CreditPayment,
	// isWithin will return false (see isWithin for details). So instead we
	// resolve MessageIsReady to whatever its actually supposed to be before
	// checking for MessageForTransaction. That way we'll see the CreditPayment.
	//
	// TODO FIXME This is kind of screwy and indicative of a design flaw. The
	// executor system makes assumptions/enforces requirements around how
	// messages are bundled together. But there are various edge cases, such as
	// situations that produce MessageIsReady, that complicate matters. So
	// instead of having a bunch of edge cases that need to be dealt with,
	// either production of MessageIsReady should be changed to match the normal
	// process, or the normal process should be updated to be less fragile, or
	// both.
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic, internal.MessageTypeNetworkUpdate) {
		var signed bool
		for _, msg := range ctx.messages {
			// Resolve MessageIsReady
			if ready, ok := msg.(*internal.MessageIsReady); ok {
				m, err := batch.Message(ready.TxID.Hash()).Main().Get()
				if err != nil {
					return nil, errors.InternalError.WithFormat("load ready message: %w", err)
				}
				msg = m
			}

			msg, ok := messaging.UnwrapAs[messaging.MessageForTransaction](msg)
			if !ok {
				continue
			}
			if msg.GetTxID().Hash() != txn.Hash() {
				continue
			}
			signed = true
		}
		if !signed {
			return nil, errors.BadRequest.With("transaction is not signed")
		}
	}

	// TODO Can we remove this or do it a better way?
	if txn.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return nil, errors.BadRequest.WithFormat("a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)
	}

	// Resolve a remote transaction to the locally stored copy (or not)
	if resolve {
		_, err := x.resolveTransaction(batch, txn)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

	} else if isRemote {
		return txn, nil
	}

	// Verify the transaction type is recognized
	//
	// If the transaction is borked, the transaction type is probably invalid,
	// so check that first. "Invalid transaction type" is a more useful error
	// than "invalid signature" if the real error is the transaction got borked.
	_, ok = ctx.Executor.executors[txn.Transaction.Body.Type()]
	if !ok {
		return nil, errors.BadRequest.WithFormat("unsupported transaction type: %v", txn.Transaction.Body.Type())
	}

	// Verify proper wrapping
	err := x.checkWrapper(ctx, txn.Transaction)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return txn, nil
}

func (TransactionMessage) checkWrapper(ctx *MessageContext, txn *protocol.Transaction) error {
	if ctx.isWithin(internal.MessageTypeMessageIsReady) {
		return nil
	}

	// Only allow synthetic transactions within a synthetic message, anchor
	// transactions within a block anchor, and don't allow other transactions to
	// be wrapped in either
	if ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypePseudoSynthetic) {
		if !txn.Body.Type().IsSynthetic() {
			return errors.BadRequest.WithFormat("a synthetic message cannot carry a %v transaction", txn.Body.Type())
		}
	} else if ctx.isWithin(messaging.MessageTypeBlockAnchor) {
		if !txn.Body.Type().IsAnchor() {
			return errors.BadRequest.WithFormat("a block anchor cannot carry a %v transaction", txn.Body.Type())
		}
	} else {
		if typ := txn.Body.Type(); typ.IsSynthetic() || typ.IsAnchor() {
			return errors.BadRequest.WithFormat("a non-synthetic message cannot carry a %v transaction", txn.Body.Type())
		}
	}
	return nil
}

func (x TransactionMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Process the transaction
	shouldExecute := ctx.shouldExecuteTransaction()
	txn, err := x.check(batch, ctx, shouldExecute)
	if err == nil {
		// Record the message if it is valid and not remote
		if txn.Transaction.Body.Type() != protocol.TransactionTypeRemote {
			err = batch.Message(ctx.message.Hash()).Main().Put(ctx.message)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("store message: %w", err)
			}
		}

		// Execute if it's time
		if shouldExecute {
			var s2 *protocol.TransactionStatus
			s2, err = x.executeTransaction(batch, ctx.txnWith(txn.Transaction))
			if err == nil && shouldExecute {
				s2.TxID = ctx.message.ID()
				s2.Received = status.Received
				status = s2
			}
		}
	}

	// Update the status
	switch {
	case err == nil:
		// DO NOT update the status code. The status code should only be updated
		// when the transaction is executed.

	case errors.Code(err).IsClientError():
		status.Set(err)

	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Transaction2(ctx.message.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	return status, nil
}

func (TransactionMessage) resolveTransaction(batch *database.Batch, msg *messaging.TransactionMessage) (bool, error) {
	isRemote := msg.GetTransaction().Body.Type() == protocol.TransactionTypeRemote
	s, err := batch.Message(msg.ID().Hash()).Main().Get()
	s2, isTxn := s.(*messaging.TransactionMessage)
	switch {
	case errors.Is(err, errors.NotFound) && !isRemote:
		// Store the transaction
		return true, nil

	case err != nil:
		// Unknown error or remote transaction with no local copy
		return false, errors.UnknownError.WithFormat("load transaction: %w", err)

	case !isTxn:
		// It's not a transaction
		return false, errors.BadRequest.With("not a transaction")

	case isRemote:
		// Resolved remote transaction from database
		msg.Transaction = s2.GetTransaction()
		return false, nil

	case s2.Equal(msg):
		// Transaction has already been recorded
		return false, nil

	default:
		// This should be impossible
		return false, errors.InternalError.WithFormat("submitted transaction does not match the locally stored transaction")
	}
}

func (x TransactionMessage) executeTransaction(batch *database.Batch, ctx *TransactionContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Record when the transaction is received
	status, err := batch.Transaction(ctx.transaction.GetHash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if status.Received == 0 {
		status.Received = ctx.Block.Index
		err = batch.Transaction(ctx.transaction.GetHash()).Status().Put(status)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Do not process the transaction if it has already been delivered
	if status.Delivered() {
		return status, nil
	}

	status, state, err := ctx.processTransaction(batch)
	if err != nil {
		return nil, err
	}

	kv := []interface{}{
		"block", ctx.Block.Index,
		"type", ctx.transaction.Body.Type(),
		"code", status.Code,
		"txn-hash", logging.AsHex(ctx.transaction.GetHash()).Slice(0, 4),
		"principal", ctx.transaction.Header.Principal,
	}
	if status.Error != nil {
		kv = append(kv, "error", status.Error)
		if ctx.pass > 0 {
			ctx.Executor.logger.Info("Additional transaction failed", kv...)
		} else {
			ctx.Executor.logger.Info("Transaction failed", kv...)
		}
	} else if status.Pending() {
		if ctx.pass > 0 {
			ctx.Executor.logger.Debug("Additional transaction pending", kv...)
		} else {
			ctx.Executor.logger.Debug("Transaction pending", kv...)
		}
	} else {
		fn := ctx.Executor.logger.Debug
		switch ctx.transaction.Body.Type() {
		case protocol.TransactionTypeDirectoryAnchor,
			protocol.TransactionTypeBlockValidatorAnchor:
			fn = ctx.Executor.logger.Info
			kv = append(kv, "module", "anchoring")
		}
		if ctx.pass > 0 {
			fn("Additional transaction succeeded", kv...)
		} else {
			fn("Transaction succeeded", kv...)
		}
	}

	err = x.postProcess(batch, ctx, state, status.Delivered())
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process scheduled events
	if status.Delivered() {
		if body, ok := ctx.transaction.Body.(*protocol.DirectoryAnchor); ok {
			err = x.processDirAnchor(batch, ctx.MessageContext, body)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
		}
	}

	return status, nil
}

func (x TransactionMessage) postProcess(batch *database.Batch, ctx *TransactionContext, state *chain.ProcessTransactionState, delivered bool) error {
	// Calculate refunds
	var swos []protocol.SyntheticTransaction
	for _, newTxn := range state.ProducedTxns {
		if swo, ok := newTxn.Body.(protocol.SyntheticTransaction); ok {
			swos = append(swos, swo)
		}
	}

	var err error
	if len(swos) > 0 {
		err = ctx.Executor.setSyntheticOrigin(batch, ctx.transaction, swos)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Clear votes and payments
	if delivered {
		txn := batch.Account(ctx.transaction.Header.Principal).
			Transaction(ctx.transaction.ID().Hash())

		err = txn.Payments().Put(nil)
		if err != nil {
			return errors.UnknownError.WithFormat("clear payments: %w", err)
		}

		err = txn.Votes().Put(nil)
		if err != nil {
			return errors.UnknownError.WithFormat("clear votes: %w", err)
		}
	}

	for _, newTxn := range state.ProducedTxns {
		msg := &messaging.TransactionMessage{Transaction: newTxn}
		err = ctx.didProduce(batch, newTxn.Header.Principal, msg)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	ctx.additional = append(ctx.additional, state.AdditionalMessages...)
	ctx.state.Set(ctx.transaction.ID().Hash(), state)
	return nil
}

func (x TransactionMessage) processDirAnchor(batch *database.Batch, ctx *MessageContext, anchor *protocol.DirectoryAnchor) error {
	// TODO Move to transaction executor once it has been refactored

	// Process minor block events
	events := batch.Account(ctx.Executor.Describe.Ledger()).Events()
	blocks, err := getBlocksWithEvents(events.Minor().Blocks(), anchor.MinorBlockIndex)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Release held authority signatures
	err = ctx.releaseHeldAuthSigs(batch, blocks)
	if err != nil {
		return errors.UnknownError.WithFormat("release held authority signatures: %w", err)
	}

	// Process major block events
	if anchor.MajorBlockIndex == 0 || ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() {
		blocks = nil
	} else {
		blocks, err = getBlocksWithEvents(events.Major().Blocks(), anchor.MajorBlockIndex)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Expire pending transactions
	messages, err := ctx.expirePendingTransactions(batch, blocks)
	if err != nil {
		return errors.UnknownError.WithFormat("expire pending transactions: %w", err)
	}
	for _, msg := range messages {
		ctx.queueAdditional(msg)
	}

	return nil
}

func getBlocksWithEvents(record values.Set[uint64], height uint64) ([]uint64, error) {
	blocks, err := record.Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load on-hold minor block list: %w", err)
	}

	// Find the block in the list
	i, found := sortutil.Search(blocks, func(b uint64) int { return int(b) - int(height) })
	if found {
		i++ // If there is an exact match, we want the index _after_ it
	}
	if i == 0 {
		return nil, nil
	}

	// Truncate the list
	err = record.Put(blocks[i:])
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store on-hold minor block list: %w", err)
	}

	// Return the truncated blocks
	return blocks[:i], nil
}

func (b *Block) expirePendingTransactions(batch *database.Batch, blocks []uint64) ([]messaging.Message, error) {
	limit := b.Executor.globals.Active.Globals.Limits.EventsPerBlock
	if limit == 0 {
		limit = 100
	}

	var backlog []*url.TxID
	var messages []messaging.Message
	for _, block := range blocks {
		// Load the list
		record := batch.Account(b.Executor.Describe.Ledger()).
			Events().
			Major().
			Pending(block)
		ids, err := record.Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load pending transactions: %w", err)
		}

		// And erase it
		err = record.Put(nil)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("reset pending transactions: %w", err)
		}

		// Have we or will we exceed the limit?
		if n := int(limit) - b.State.Events; n <= 0 {
			backlog = append(backlog, ids...)
			continue
		} else if n < len(ids) {
			backlog = append(backlog, ids[n:]...)
			ids = ids[:n]
		}

		b.State.Events += len(ids)

		for _, id := range ids {
			messages = append(messages, &internal.ExpiredTransaction{TxID: id})
		}
	}

	// If we didn't finish everything, add it to the backlog
	if len(backlog) == 0 {
		return messages, nil
	}

	err := batch.Account(b.Executor.Describe.Ledger()).
		Events().
		Backlog().
		Expired().
		Add(backlog...)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store expired transaction backlog: %w", err)
	}
	return messages, nil
}

func (b *Block) processEvents() error {
	if majorBlockIndex, _, ok := b.didOpenMajorBlock(); ok && b.Executor.globals.Active.ExecutorVersion.V2VandenbergEnabled() {
		// Process major block events
		events := b.Batch.Account(b.Executor.Describe.Ledger()).Events()
		blocks, err := getBlocksWithEvents(events.Major().Blocks(), majorBlockIndex)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Expire pending transactions
		msgs, err := b.expirePendingTransactions(b.Batch, blocks)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Claim we're on pass 1 so that internal messages are allowed
		_, err = b.processMessages(msgs, 1)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	n := int(b.Executor.globals.Active.Globals.Limits.EventsPerBlock)
	if n == 0 {
		n = 100
	}
	n -= b.State.Events
	if n <= 0 {
		return nil
	}

	// Load the backlog
	batch := b.Batch
	record := batch.Account(b.Executor.Describe.Ledger()).
		Events().
		Backlog().
		Expired()
	backlog, err := record.Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load expired transaction backlog: %w", err)
	}
	if len(backlog) == 0 {
		return nil
	}
	if n > len(backlog) {
		n = len(backlog)
	}

	// Store the rest
	err = record.Put(backlog[n:])
	if err != nil {
		return errors.UnknownError.WithFormat("load expired transaction backlog: %w", err)
	}

	d := new(bundle)
	d.Block = b
	d.state = orderedMap[[32]byte, *chain.ProcessTransactionState]{cmp: func(u, v [32]byte) int { return bytes.Compare(u[:], v[:]) }}

	// Process N items
	msgs := make([]messaging.Message, n)
	for i, id := range backlog[:n] {
		msgs[i] = &internal.ExpiredTransaction{TxID: id}
	}

	// Claim we're on pass 1 so that internal messages are allowed
	_, err = b.processMessages(msgs, 1)
	return errors.UnknownError.Wrap(err)
}

// ExpiredTransaction expires a transaction
type ExpiredTransaction struct{}

func (ExpiredTransaction) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, errors.InternalError.With("invalid attempt to validate an internal message")
}

func (x ExpiredTransaction) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, ok := ctx.message.(*internal.ExpiredTransaction)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeExpiredTransaction, ctx.message.Type())
	}

	err := x.expireTransaction(batch, ctx, msg)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = x.eraseSignatures(batch, ctx, msg)
	return nil, errors.UnknownError.Wrap(err)
}

func (x ExpiredTransaction) expireTransaction(batch *database.Batch, ctx *MessageContext, msg *internal.ExpiredTransaction) error {
	// If the transaction has been executed (which erases the credit
	// payments), skip it
	isInit, _, err := transactionIsInitiated(batch, msg.TxID)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !isInit {
		return nil
	}

	// Load it
	var txn *messaging.TransactionMessage
	err = batch.Message(msg.TxID.Hash()).Main().GetAs(&txn)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		// This should not happen but returning an error would be bad
		ctx.Executor.logger.Error("Missing pending transaction", "id", msg.TxID)
		return nil
	default:
		return errors.UnknownError.WithFormat("load pending transaction: %w", err)
	}
	if ctx.message == nil {
		ctx.message = msg
	}

	// If the account in the expiring ID is not the principal, skip it (because
	// it's a signature set expiration)
	if ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() && !msg.TxID.Account().Equal(txn.Transaction.Header.Principal) {
		return nil
	}

	ctx2 := ctx.txnWith(txn.Transaction)
	_, state, err := ctx2.recordFailedTransaction(batch, &chain.Delivery{Transaction: txn.Transaction}, errors.Expired)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	ctx.State.MergeTransaction(state)

	err = TransactionMessage{}.postProcess(batch, ctx2, state, true)
	return errors.UnknownError.Wrap(err)
}

func (x ExpiredTransaction) eraseSignatures(batch *database.Batch, ctx *MessageContext, msg *internal.ExpiredTransaction) error {
	// Load the account
	account, err := batch.Account(msg.TxID.Account()).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load account: %w", err)
	}

	// Is it an authority?
	if _, ok := account.(protocol.Authority); !ok {
		return nil
	}

	// Load the transaction
	var txn *messaging.TransactionMessage
	err = batch.Message(msg.TxID.Hash()).Main().GetAs(&txn)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		// This should not happen but returning an error would be bad
		ctx.Executor.logger.Error("Missing pending transaction", "id", msg.TxID)
		return nil
	default:
		return errors.UnknownError.WithFormat("load pending transaction: %w", err)
	}

	// Clear the signature set
	err = clearActiveSignatures(batch, account.GetUrl(), txn.ID())
	return errors.UnknownError.Wrap(err)
}
