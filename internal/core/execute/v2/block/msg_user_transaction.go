// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[UserTransaction](&messageExecutors, messaging.MessageTypeUserTransaction)
}

// UserTransaction records the transaction but does not execute it. Transactions
// are executed in response to _authority signature_ messages, not user
// transaction messages.
type UserTransaction struct{}

func (x UserTransaction) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	txn, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	delivery := new(chain.Delivery)
	delivery.Transaction = txn.Transaction
	delivery.Internal = ctx.isWithin(internal.MessageTypeNetworkUpdate)
	if ctx.isWithin(messaging.MessageTypeSequenced) {
		delivery.Sequence, err = x.getSequence(ctx)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	var signed bool
	for _, msg := range ctx.messages {
		msg, ok := messaging.UnwrapAs[messaging.MessageForTransaction](msg)
		if !ok {
			continue
		}

		// Handles special types of 'signatures'
		signed = true

		sig, ok := msg.(*messaging.UserSignature)
		if !ok ||
			sig.Signature.Type() == protocol.SignatureTypeAuthority ||
			sig.TxID.Hash() != txn.Hash() {
			continue
		}

		delivery.Signatures = append(delivery.Signatures, sig.Signature)
	}
	if !signed {
		return nil, errors.BadRequest.With("transaction is not signed")
	}

	// For now, don't validate the transaction that is sent along with an
	// authority signature/signature request/credit payment
	if len(delivery.Signatures) == 0 {
		return nil, nil
	}

	result, err := ctx.Executor.ValidateEnvelope(batch, delivery)
	switch {
	case err == nil:
		s := new(protocol.TransactionStatus)
		s.TxID = ctx.message.ID()
		s.Result = result
		return s, nil
	case errors.Code(err).IsKnownError():
		return nil, err
	default:
		// If the error is not an Error, assume it is a client error, produced
		// by old code not yet using the status codes
		return nil, errors.BadRequest.Wrap(err)
	}
}

func (UserTransaction) check(batch *database.Batch, ctx *MessageContext) (*messaging.UserTransaction, error) {
	txn, ok := ctx.message.(*messaging.UserTransaction)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeUserTransaction, ctx.message.Type())
	}

	if txn.Transaction == nil {
		return nil, errors.BadRequest.With("missing transaction")
	}
	if txn.Transaction.Body == nil {
		return nil, errors.BadRequest.With("missing transaction body")
	}

	// TODO Can we remove this or do it a better way?
	if txn.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return nil, errors.BadRequest.WithFormat("a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)
	}

	return txn, nil
}

func (x UserTransaction) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	batch = batch.Begin(true)
	defer batch.Discard()

	txn, err := x.check(batch, ctx)
	switch {
	case err == nil:
		// Ok

	case errors.Code(err).IsClientError():
		status := new(protocol.TransactionStatus)
		status.Received = ctx.Block.Index
		status.TxID = ctx.message.ID()
		status.Set(err)
		err = batch.Transaction2(txn.Hash()).Status().Put(status)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store status: %w", err)
		}
		return status, nil

	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	// Resolve and store the transaction
	ctx2 := ctx.txnWith(txn.Transaction)
	ctx2.transaction, err = x.storeTransaction(batch, ctx, txn)
	if err != nil {
		if err, ok := err.(*errors.Error); ok && err.Code.IsClientError() {
			return protocol.NewErrorStatus(txn.ID(), err), nil
		}
		return nil, errors.UnknownError.Wrap(err)
	}

	// Check the transaction, but only if it was not internally produced
	if !ctx.isWithin(internal.MessageTypeMessageIsReady, internal.MessageTypeNetworkUpdate) {
		st, err := x.checkTransaction(batch, ctx2)
		if err != nil || st.Failed() {
			return st, err
		}
	}

	// Execute the transaction, but ONLY if this is a nested context - DO NOT
	// attempt to execute a bare user transaction
	var status *protocol.TransactionStatus
	if ctx.shouldExecuteTransaction() {
		status, err = x.executeTransaction(batch, ctx2)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x UserTransaction) storeTransaction(batch *database.Batch, ctx *MessageContext, msg *messaging.UserTransaction) (*protocol.Transaction, error) {
	txn := msg.GetTransaction()
	record := batch.Message(txn.ID().Hash())

	// Validate the synthetic transaction header
	if typ := txn.Body.Type(); (typ.IsSynthetic() || typ.IsAnchor()) && !ctx.isWithin(messaging.MessageTypeSequenced) {
		return nil, errors.BadRequest.WithFormat("a %v transaction must be sequenced", typ)
	}

	new, err := x.resolveTransaction(batch, msg)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !new {
		return txn, nil
	}

	// If we reach this point, Validate should have verified that there is a
	// signer that can be charged for this recording
	err = record.Main().Put(msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	return txn, nil
}

func (UserTransaction) resolveTransaction(batch *database.Batch, msg *messaging.UserTransaction) (bool, error) {
	isRemote := msg.GetTransaction().Body.Type() == protocol.TransactionTypeRemote
	s, err := batch.Message(msg.ID().Hash()).Main().Get()
	s2, isTxn := s.(*messaging.UserTransaction)
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

func (UserTransaction) checkTransaction(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	// Ensure the transaction is signed, is synthetic, or was internally queued
	if !ctx.isWithin(messaging.MessageTypeSynthetic, messaging.MessageTypeBlockAnchor, messaging.MessageTypeUserSignature) {
		var signed bool
		for _, other := range ctx.messages {
		again:
			switch m := other.(type) {
			case messaging.MessageForTransaction:
				if m.GetTxID().Hash() == ctx.transaction.ID().Hash() {
					signed = true
				}
			case interface{ Unwrap() messaging.Message }:
				other = m.Unwrap()
				goto again
			}
			if signed {
				break
			}
		}
		if !signed {
			return protocol.NewErrorStatus(ctx.transaction.ID(), errors.BadRequest.WithFormat("%v is not signed", ctx.transaction.ID())), nil
		}
	}

	// Only allow synthetic transactions within a synthetic message, anchor
	// transactions within a block anchor, and don't allow other transactions to
	// be wrapped in either
	if ctx.isWithin(messaging.MessageTypeSynthetic) {
		if !ctx.transaction.Body.Type().IsSynthetic() {
			return protocol.NewErrorStatus(ctx.transaction.ID(), errors.BadRequest.WithFormat("a synthetic message cannot carry a %v transaction", ctx.transaction.Body.Type())), nil
		}
	} else if ctx.isWithin(messaging.MessageTypeBlockAnchor) {
		if !ctx.transaction.Body.Type().IsAnchor() {
			return protocol.NewErrorStatus(ctx.transaction.ID(), errors.BadRequest.WithFormat("a block anchor cannot carry a %v transaction", ctx.transaction.Body.Type())), nil
		}
	} else {
		if typ := ctx.transaction.Body.Type(); typ.IsSynthetic() || typ.IsAnchor() {
			return protocol.NewErrorStatus(ctx.transaction.ID(), errors.BadRequest.WithFormat("a non-synthetic message cannot carry a %v transaction", ctx.transaction.Body.Type())), nil
		}
	}

	return nil, nil
}

func (UserTransaction) getSequence(ctx *MessageContext) (*messaging.SequencedMessage, error) {
	seq, ok := getMessageContextAncestor[*messaging.SequencedMessage](ctx)
	if !ok {
		return nil, errors.InternalError.With("not within a sequence message")
	}
	if seq.Message != ctx.message {
		return nil, errors.InternalError.With("within a sequence message belonging to a different message")
	}
	return seq, nil
}

func (x UserTransaction) executeTransaction(batch *database.Batch, ctx *TransactionContext) (*protocol.TransactionStatus, error) {
	batch = batch.Begin(true)
	defer batch.Discard()

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

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit batch: %w", err)
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

	for _, newTxn := range state.ProducedTxns {
		msg := &messaging.UserTransaction{Transaction: newTxn}
		ctx.didProduce(newTxn.Header.Principal, msg)
	}
	ctx.additional = append(ctx.additional, state.AdditionalMessages...)
	ctx.state.Set(ctx.transaction.ID().Hash(), state)
	return status, nil
}
