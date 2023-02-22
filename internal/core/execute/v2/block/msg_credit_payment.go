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

func (x CreditPayment) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	pay, ok := ctx.message.(*messaging.CreditPayment)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeCreditPayment, ctx.message.Type())
	}

	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		return protocol.NewErrorStatus(pay.ID(), errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", pay.Type())), nil
	}

	// Basic validation
	if pay.Payer == nil {
		return protocol.NewErrorStatus(pay.ID(), errors.BadRequest.With("missing authority")), nil
	}
	if pay.TxID == nil {
		return protocol.NewErrorStatus(pay.ID(), errors.BadRequest.With("missing transaction ID")), nil
	}
	if pay.Cause == nil {
		return protocol.NewErrorStatus(pay.ID(), errors.BadRequest.With("missing cause")), nil
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(pay.Hash(), new(chain.ProcessTransactionState))

	batch = batch.Begin(true)
	defer batch.Discard()

	// If the message has already been processed, return its recorded status
	status, err := batch.Transaction2(pay.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Delivered() {
		return status, nil
	}

	// Record the payment
	status = new(protocol.TransactionStatus)
	status.Received = ctx.Block.Index
	status.TxID = pay.ID()

	txn, err := x.record(batch, ctx, pay)
	var err2 *errors.Error
	switch {
	case err == nil:
		status.Code = errors.Delivered

	case errors.As(err, &err2) && err2.Code.IsClientError():
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
	h := pay.Hash()
	err = batch.Transaction(h[:]).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	// The transaction may be ready so process it
	_, err = ctx.callMessageExecutor(batch, &messaging.UserTransaction{Transaction: txn})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	return status, errors.UnknownError.Wrap(err)
}

func (CreditPayment) record(batch *database.Batch, ctx *MessageContext, pay *messaging.CreditPayment) (*protocol.Transaction, error) {
	// TODO Record payment

	var txn messaging.MessageWithTransaction
	err := batch.Message(pay.TxID.Hash()).Main().GetAs(&txn)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Record the initiator on the transaction status
	if !pay.Initiator {
		return txn.GetTransaction(), nil
	}

	status, err := batch.Transaction2(pay.TxID.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction status: %w", err)
	}
	if status.Initiator == nil {
		status.Initiator = pay.Payer
		err = batch.Transaction2(pay.TxID.Hash()).Status().Put(status)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store transaction status: %w", err)
		}
	} else if !status.Initiator.Equal(pay.Payer) {
		return nil, errors.Conflict.WithFormat("conflicting initiator for %v", pay.TxID)
	}

	return txn.GetTransaction(), nil
}
