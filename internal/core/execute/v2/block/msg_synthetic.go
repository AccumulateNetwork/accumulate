// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SyntheticMessage](&messageExecutors, messaging.MessageTypeSynthetic)
}

// SyntheticMessage records the synthetic transaction but does not execute
// it.
type SyntheticMessage struct{}

func (x SyntheticMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// Check the wrapper
	syn, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate the inner message
	_, err = ctx.callMessageValidator(batch, syn.Message)
	return nil, errors.UnknownError.Wrap(err)
}

func (SyntheticMessage) check(batch *database.Batch, ctx *MessageContext) (*messaging.SyntheticMessage, error) {
	syn, ok := ctx.message.(*messaging.SyntheticMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSynthetic, ctx.message.Type())
	}

	// Basic validation
	if syn.Message == nil {
		return nil, errors.BadRequest.With("missing message")
	}
	if syn.Proof == nil {
		return nil, errors.BadRequest.With("missing proof")
	}
	if syn.Proof.Receipt == nil {
		return nil, errors.BadRequest.With("missing proof receipt")
	}
	if syn.Proof.Anchor == nil || syn.Proof.Anchor.Account == nil {
		return nil, errors.BadRequest.With("missing proof metadata")
	}
	if !syn.Proof.Receipt.Validate() {
		return nil, errors.BadRequest.With("proof is invalid")
	}

	// A synthetic message must be sequenced (may change in the future)
	seq, ok := syn.Message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.BadRequest.With("a synthetic message must be sequenced")
	}

	// Verify the proof starts with the transaction hash
	h := syn.Message.ID().Hash()
	if !bytes.Equal(h[:], syn.Proof.Receipt.Start) {
		return nil, errors.BadRequest.WithFormat("invalid proof start: expected %x, got %x", h, syn.Proof.Receipt.Start)
	}

	// Verify the proof ends with a DN anchor
	_, err := batch.Account(ctx.Executor.Describe.AnchorPool()).AnchorChain(protocol.Directory).Root().IndexOf(syn.Proof.Receipt.Anchor)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, errors.BadRequest.WithFormat("invalid proof anchor: %x is not a known directory anchor", syn.Proof.Receipt.Anchor)
	default:
		return nil, errors.UnknownError.WithFormat("search for directory anchor %x: %w", syn.Proof.Receipt.Anchor, err)
	}

	// Verify the message within the sequenced message is an allowed type
	switch seq.Message.Type() {
	case messaging.MessageTypeUserTransaction,
		messaging.MessageTypeUserSignature,
		messaging.MessageTypeSignatureRequest,
		messaging.MessageTypeCreditPayment:
		// Allowed

	default:
		return nil, errors.BadRequest.WithFormat("a synthetic message cannot carry a %v message", seq.Message.Type())
	}

	return syn, nil
}

func (x SyntheticMessage) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
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

	syn, err := x.check(batch, ctx)
	if err == nil {
		_, err = ctx.callMessageExecutor(batch, syn.Message)
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

	// Record the synthetic message and it's cause/produced relation
	err = batch.Message(syn.Hash()).Main().Put(syn)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	err = batch.Message(syn.Hash()).Produced().Add(syn.Message.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message produced: %w", err)
	}

	err = batch.Message(syn.Message.Hash()).Cause().Add(syn.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message cause: %w", err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}
