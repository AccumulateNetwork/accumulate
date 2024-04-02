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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[NetworkUpdate](&messageExecutors,
		internal.MessageTypeNetworkUpdate)
	registerConditionalExec[NetworkUpdate](&messageExecutors,
		func(ctx *MessageContext) bool { return ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() },
		messaging.MessageTypeNetworkUpdate)
}

// NetworkUpdate constructs a transaction for the network update and queues it
// for processing.
type NetworkUpdate struct{}

func (x NetworkUpdate) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	switch ctx.message.(type) {
	case *internal.NetworkUpdate:
		return nil, errors.InternalError.With("invalid attempt to validate an internal message")
	case *messaging.NetworkUpdate:
		msg := ctx.message.(*messaging.NetworkUpdate)
		err := x.check(batch, ctx, msg)
		return nil, err
	default:
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeNetworkUpdate, ctx.message.Type())
	}

}

func (NetworkUpdate) check(batch *database.Batch, ctx *MessageContext, msg *messaging.NetworkUpdate) error {
	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
		return errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", msg.Type())
	}

	return nil
}

func (x NetworkUpdate) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	switch msg := ctx.message.(type) {
	case *internal.NetworkUpdate:
		return x.processOld(batch, ctx, msg)
	case *messaging.NetworkUpdate:
		// Ok
	default:
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeNetworkUpdate, ctx.message.Type())
	}

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
	msg := ctx.message.(*messaging.NetworkUpdate)
	err = x.check(batch, ctx, msg)
	if err == nil {
		x.process(ctx, msg)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !status.Code.Success() {
		return status, nil
	}
	return status, errors.UnknownError.Wrap(err)
}

func (NetworkUpdate) process(ctx *MessageContext, msg *messaging.NetworkUpdate) {
	// It's pretty roundabout for a messaging.NetworkUpdate to queue an
	// internal.NetworkUpdate that does the actual work. The motivation is that
	// network updates were previously distributed as part of a DirectoryAnchor,
	// and that transaction queued an internal network update. Now that network
	// updates are transmitted in their own message, separately from the
	// directory anchor, implementing it this way is a smaller overall change
	// than executing the updates in this loop. And a smaller update means less
	// bugs.
	for _, update := range msg.Accounts {
		var account *url.URL
		switch update.Name {
		case protocol.Operators:
			account = ctx.Executor.Describe.OperatorsPage()
		default:
			account = ctx.Executor.Describe.NodeUrl(update.Name)
		}

		ctx.queueAdditional(&internal.NetworkUpdate{
			Cause:   ctx.message.Hash(),
			Account: account,
			Body:    update.Body,
		})
	}
}

func (NetworkUpdate) processOld(batch *database.Batch, ctx *MessageContext, msg *internal.NetworkUpdate) (*protocol.TransactionStatus, error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = msg.Account
	txn.Header.Initiator = msg.Cause
	txn.Body = msg.Body

	batch = batch.Begin(true)
	defer batch.Discard()

	// Record that the cause produced this update
	err := batch.Transaction(msg.Cause[:]).Produced().Add(txn.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("update cause: %w", err)
	}

	// Store the transaction
	err = batch.Message(txn.ID().Hash()).Main().Put(&messaging.TransactionMessage{Transaction: txn})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	// Store a fake payment
	pay := new(messaging.CreditPayment)
	pay.Payer = protocol.DnUrl().JoinPath(protocol.Network)
	pay.Cause = pay.Payer.WithTxID(msg.Cause)
	pay.Initiator = true
	pay.TxID = txn.ID()
	err = batch.Message(pay.Hash()).Main().Put(pay)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store payment: %w", err)
	}
	err = batch.Message(pay.Hash()).Cause().Add(pay.Cause)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store payment cause: %w", err)
	}

	err = batch.Account(msg.Account).
		Transaction(txn.ID().Hash()).
		Payments().
		Add(pay.Hash())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store payment hash: %w", err)
	}

	// Execute the transaction
	st, err := ctx.callMessageExecutor(batch, &messaging.TransactionMessage{Transaction: txn})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return st, nil
}
