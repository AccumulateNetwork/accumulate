// Copyright 2025 The Accumulate Authors
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
	registerSimpleExec[FeeEscrowPayment](&messageExecutors, messaging.MessageTypeFeeEscrowPayment)
}

// FeeEscrowPayment processes a fee escrow payment (AIP-50)
type FeeEscrowPayment struct{}

func (x FeeEscrowPayment) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	_, _, err := x.check(batch, ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (FeeEscrowPayment) check(batch *database.Batch, ctx *MessageContext) (*messaging.FeeEscrowPayment, *protocol.Transaction, error) {
	pay, ok := ctx.message.(*messaging.FeeEscrowPayment)
	if !ok {
		return nil, nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeFeeEscrowPayment, ctx.message.Type())
	}

	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
		return nil, nil, errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", pay.Type())
	}

	// Basic validation
	if pay.Payer == nil {
		return nil, nil, errors.BadRequest.With("missing payer")
	}
	if pay.TxID == nil {
		return nil, nil, errors.BadRequest.With("missing transaction ID")
	}
	if pay.Cause == nil {
		return nil, nil, errors.BadRequest.With("missing cause")
	}
	if pay.Token == nil {
		return nil, nil, errors.BadRequest.With("missing token")
	}
	if pay.EscrowAccount == nil {
		return nil, nil, errors.BadRequest.With("missing escrow account")
	}

	// Load the transaction
	txn, err := ctx.getTransaction(batch, pay.TxID.Hash())
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Verify the fee index is valid
	if int(pay.FeeIndex) >= len(txn.Header.UserFees) {
		return nil, nil, errors.BadRequest.WithFormat("invalid fee index: %d (transaction has %d user fees)", pay.FeeIndex, len(txn.Header.UserFees))
	}

	return pay, txn, nil
}

func (x FeeEscrowPayment) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message
	pay, txn, err := x.check(batch, ctx)
	if err == nil {
		err = x.process(batch, ctx, pay, txn)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (FeeEscrowPayment) process(batch *database.Batch, ctx *MessageContext, pay *messaging.FeeEscrowPayment, txn *protocol.Transaction) error {
	// Record the escrow in transaction metadata
	acctTxn := batch.Account(pay.TxID.Account()).Transaction(pay.TxID.Hash())

	// Record in history
	err := acctTxn.RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	// Get the fee spec from transaction
	fee := txn.Header.UserFees[pay.FeeIndex]

	// Store escrow entry
	entry := &protocol.FeeEscrowEntry{
		Index:         pay.FeeIndex,
		Amount:        pay.Amount,
		Token:         pay.Token,
		Payer:         pay.Payer,
		Recipient:     fee.Recipient,
		EscrowAccount: pay.EscrowAccount,
		MessageHash:   pay.Hash(),
	}

	err = acctTxn.FeeEscrows().Add(entry)
	if err != nil {
		return errors.UnknownError.WithFormat("store escrow entry: %w", err)
	}

	return nil
}
