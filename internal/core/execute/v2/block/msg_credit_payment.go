// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
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
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
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

func (x CreditPayment) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message and the transaction
	pay, txn, err := x.check(batch, ctx)
	if err == nil {
		err = x.process(batch, ctx, pay)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !status.Code.Success() {
		return status, nil
	}

	_, err = ctx.callMessageExecutor(batch, &messaging.TransactionMessage{Transaction: txn})
	return status, errors.UnknownError.Wrap(err)
}

func (CreditPayment) process(batch *database.Batch, ctx *MessageContext, pay *messaging.CreditPayment) error {
	// Record the payment
	acctTxn := batch.Account(pay.TxID.Account()).Transaction(pay.TxID.Hash())
	err := acctTxn.RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	err = acctTxn.Payments().Add(pay.Hash())
	if err != nil {
		return errors.UnknownError.WithFormat("record payment: %w", err)
	}

	if !pay.Initiator {
		return nil
	}

	// If the transaction is being initiated, mark it as pending. This only
	// persists if the transaction remains pending through the end of the block.
	var txn *protocol.Transaction
	if ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		// Load the transaction
		txn, err = ctx.getTransaction(batch, pay.TxID.Hash())
		if err != nil {
			return errors.UnknownError.WithFormat("load transaction: %w", err)
		}

	} else {
		// Fake transaction
		txn = new(protocol.Transaction)
		txn.Header.Principal = pay.TxID.Account()
		txn.Body = &protocol.RemoteTransaction{Hash: pay.TxID.Hash()}
	}

	ctx.State.MarkTransactionPending(txn)
	return nil
}
