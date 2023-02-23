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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[CreditPayment](&messageExecutors, messaging.MessageTypeCreditPayment)
}

// CreditPayment processes a credit payment
type CreditPayment struct{}

func (x CreditPayment) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	_, _, err := x.check(batch, ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (CreditPayment) check(batch *database.Batch, ctx *MessageContext) (*messaging.CreditPayment, *protocol.Transaction, error) {
	pay, ok := ctx.message.(*messaging.CreditPayment)
	if !ok {
		return nil, nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeCreditPayment, ctx.message.Type())
	}

	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		return nil, nil, errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", pay.Type())
	}

	// Basic validation
	if pay.Payer == nil {
		return nil, nil, errors.BadRequest.With("missing authority")
	}
	if pay.TxID == nil {
		return nil, nil, errors.BadRequest.With("missing transaction ID")
	}
	if pay.Cause == nil {
		return nil, nil, errors.BadRequest.With("missing cause")
	}

	// Load the transaction
	txn, err := ctx.getTransaction(batch, pay.TxID.Hash())
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	return pay, txn, nil
}

func (x CreditPayment) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// If the message has already been processed, return its recorded status
	status, err := batch.Transaction2(ctx.message.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Delivered() {
		return status, nil
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	batch = batch.Begin(true)
	defer batch.Discard()

	// Process the message
	status = new(protocol.TransactionStatus)
	status.Received = ctx.Block.Index
	status.TxID = ctx.message.ID()

	pay, txn, err := x.check(batch, ctx)
	if err == nil {
		err = x.record(batch, ctx, pay)
	}

	// Update the status
	switch {
	case err == nil:
		status.Code = errors.Delivered

	case errors.Code(err).IsClientError():
		status.Set(err)

	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	// Record the message
	err = batch.Message(pay.Hash()).Main().Put(pay)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	err = batch.Message(pay.Hash()).Cause().Add(pay.TxID)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("add cause: %w", err)
	}

	// Record the status
	err = batch.Transaction2(pay.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	// The transaction may be ready so process it
	if !status.Failed() {
		_, err = ctx.callMessageExecutor(batch, &messaging.UserTransaction{Transaction: txn})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	err = batch.Commit()
	return status, errors.UnknownError.Wrap(err)
}

func (CreditPayment) record(batch *database.Batch, ctx *MessageContext, pay *messaging.CreditPayment) error {
	// TODO Record payment

	// Record the initiator on the transaction status
	if !pay.Initiator {
		return nil
	}

	status, err := batch.Transaction2(pay.TxID.Hash()).Status().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load status: %w", err)
	}
	if err != nil {
		return errors.UnknownError.WithFormat("load transaction status: %w", err)
	}
	if status.Initiator == nil {
		status.Initiator = pay.Payer
		err = batch.Transaction2(pay.TxID.Hash()).Status().Put(status)
		if err != nil {
			return errors.UnknownError.WithFormat("store transaction status: %w", err)
		}
	} else if !status.Initiator.Equal(pay.Payer) {
		return errors.Conflict.WithFormat("conflicting initiator for %v", pay.TxID)
	}

	return nil
}
