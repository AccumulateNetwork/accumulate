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
		err = x.record(batch, ctx, pay)
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

func (CreditPayment) record(batch *database.Batch, ctx *MessageContext, pay *messaging.CreditPayment) error {
	// Add the message to the signature chain
	h := ctx.message.Hash()
	err := batch.Account(pay.TxID.Account()).SignatureChain().Inner().AddHash(h[:], false)
	if err != nil {
		return errors.UnknownError.WithFormat("add to signature chain: %w", err)
	}

	return batch.Account(pay.TxID.Account()).
		Transaction(pay.TxID.Hash()).
		Payments().
		Add(pay.Hash())
}
