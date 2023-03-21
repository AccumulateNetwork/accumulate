// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[TransactionMessage](&messageExecutors, messaging.MessageTypeTransaction)
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
		return status, nil
	}

	// Discard changes (why is this necessary?)
	batch = batch.Begin(false)
	defer batch.Discard()

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

	// This is a temporary hack. The transaction executors need to be updated to
	// make validation stateless. For now, if the bundle includes a user
	// signature that's local to the transaction, validate the transaction.
	if !txn.Transaction.Body.Type().IsUser() {
		return nil, nil
	}
	var hasLocalSigner bool
	for _, msg := range ctx.messages {
		sig, ok := messaging.UnwrapAs[*messaging.SignatureMessage](msg)
		if !ok || sig.Signature.Type() == protocol.SignatureTypeAuthority {
			continue
		}
		if sig.Signature.GetSigner().LocalTo(txn.Transaction.Header.Principal) {
			hasLocalSigner = true
		}
	}
	if !hasLocalSigner {
		return nil, nil
	}
	exec, ok := ctx.Executor.executors[txn.Transaction.Body.Type()]
	if !ok {
		return nil, nil
	}

	principal, err := batch.Account(txn.Transaction.Header.Principal).Main().Get()
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, errors.NotFound):
		return nil, errors.UnknownError.WithFormat("load principal: %w", err)
	default:
		val, ok := getValidator[chain.PrincipalValidator](ctx.Executor, txn.Transaction.Body.Type())
		if !ok || !val.AllowMissingPrincipal(txn.Transaction) {
			return nil, errors.NotFound.WithFormat("missing principal: %v not found", txn.Transaction.Header.Principal)
		}
	}

	st := chain.NewStateManager(&ctx.Executor.Describe, &ctx.Executor.globals.Active, ctx.Executor, batch.Begin(false), principal, txn.Transaction, ctx.Executor.logger.With("operation", "ValidateEnvelope"))
	defer st.Discard()
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
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypeNetworkUpdate) {
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
	if ctx.isWithin(messaging.MessageTypeSynthetic) {
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

func (TransactionMessage) getSequence(ctx *MessageContext) (*messaging.SequencedMessage, error) {
	seq, ok := getMessageContextAncestor[*messaging.SequencedMessage](ctx)
	if !ok {
		return nil, errors.InternalError.With("not within a sequence message")
	}
	if seq.Message != ctx.message {
		return nil, errors.InternalError.With("within a sequence message belonging to a different message")
	}
	return seq, nil
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

	delivery := &chain.Delivery{
		Transaction: ctx.transaction,
		Internal:    ctx.isWithin(internal.MessageTypeNetworkUpdate),
	}

	// Load sequence info
	if typ := ctx.transaction.Body.Type(); typ.IsSynthetic() || typ.IsAnchor() {
		delivery.Sequence, err = x.getSequence(ctx.MessageContext)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load sequence info: %w", err)
		}
	}

	status, state, err := ctx.Executor.ProcessTransaction(batch, delivery)
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

	// Calculate refunds
	var swos []protocol.SynthTxnWithOrigin
	for _, newTxn := range state.ProducedTxns {
		if swo, ok := newTxn.Body.(protocol.SynthTxnWithOrigin); ok {
			swos = append(swos, swo)
		}
	}

	if len(swos) > 0 {
		err = ctx.Executor.setSyntheticOrigin(batch, ctx.transaction, swos)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Clear votes and payments
	if status.Delivered() {
		txn := batch.Account(delivery.Transaction.Header.Principal).
			Transaction(delivery.Transaction.ID().Hash())

		err = txn.Payments().Put(nil)
		if err != nil {
			return nil, err
		}

		err = txn.Votes().Put(nil)
		if err != nil {
			return nil, err
		}
	}

	for _, newTxn := range state.ProducedTxns {
		msg := &messaging.TransactionMessage{Transaction: newTxn}
		ctx.didProduce(newTxn.Header.Principal, msg)
	}
	ctx.additional = append(ctx.additional, state.AdditionalMessages...)
	ctx.state.Set(ctx.transaction.ID().Hash(), state)
	return status, nil
}
