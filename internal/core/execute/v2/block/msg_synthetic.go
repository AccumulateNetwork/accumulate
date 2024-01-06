// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SyntheticMessage](&messageExecutors, messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic)
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

func (SyntheticMessage) check(batch *database.Batch, ctx *MessageContext) (*messaging.SynthFields, error) {
	// Using messaging.SynthFields is safer than converting one message type
	// into the other because that could lead to issues with the different Hash
	// method implementations
	var syn *messaging.SynthFields
	if !ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		msg, ok := ctx.message.(*messaging.BadSyntheticMessage)
		if !ok {
			return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBadSynthetic, ctx.message.Type())
		}
		syn = msg.Data()
	} else {
		switch msg := ctx.message.(type) {
		case *messaging.BadSyntheticMessage:
			syn = msg.Data()
		case *messaging.SyntheticMessage:
			syn = msg.Data()
		default:
			return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSynthetic, ctx.message.Type())
		}
	}

	// Basic validation
	if syn.Message == nil {
		return nil, errors.BadRequest.With("missing message")
	}
	if syn.Signature == nil {
		return nil, errors.BadRequest.With("missing signature")
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
	if !syn.Proof.Receipt.Validate(nil) {
		return nil, errors.BadRequest.With("proof is invalid")
	}

	// A synthetic message must be sequenced (may change in the future)
	seq, ok := syn.Message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.BadRequest.With("a synthetic message must be sequenced")
	}

	// Verify the signature
	h := syn.Message.Hash()
	if !syn.Signature.Verify(nil, h[:]) {
		return nil, errors.BadRequest.With("invalid signature")
	}

	// Verify the signer is a validator of this partition
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return nil, errors.BadRequest.WithFormat("signature source is not a partition")
	}

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	signer := core.AnchorSigner(&ctx.Executor.globals.Active, partition)
	_, _, ok = signer.EntryByKeyHash(syn.Signature.GetPublicKeyHash())
	if !ok {
		return nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
	}

	// Verify the proof starts with the transaction hash
	if !bytes.Equal(h[:], syn.Proof.Receipt.Start) {
		return nil, errors.BadRequest.WithFormat("invalid proof start: expected %x, got %x", h, syn.Proof.Receipt.Start)
	}

	// Don't check the anchor during validation. If we check the anchor during
	// validation, there is a race condition: partition X may receive a DN
	// anchor and submit synthetic messages to partition Y before that partition
	// receives and processes that anchor, which could cause partition Y to
	// reject the message during CheckTx. Waiting until DeliverTx to check the
	// anchor does not eliminate the race but it does significantly reduce the
	// likelihood it will strike, since partition Y will almost process the DN
	// anchor before it processes the synthetic message.

	// Verify the message within the sequenced message is an allowed type
	switch seq.Message.Type() {
	case messaging.MessageTypeTransaction,
		messaging.MessageTypeSignature,
		messaging.MessageTypeSignatureRequest,
		messaging.MessageTypeCreditPayment:
		// Allowed

	default:
		return nil, errors.BadRequest.WithFormat("a synthetic message cannot carry a %v message", seq.Message.Type())
	}

	return syn, nil
}

func (x SyntheticMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message (error is handled by the next step)
	err = x.process(batch, ctx)

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x SyntheticMessage) process(batch *database.Batch, ctx *MessageContext) error {
	// Validate
	syn, err := x.check(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify the proof ends with a DN anchor
	_, err = batch.Account(ctx.Executor.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).
		Root().
		IndexOf(syn.Proof.Receipt.Anchor)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return errors.BadRequest.WithFormat("invalid proof anchor: %x is not a known directory anchor", syn.Proof.Receipt.Anchor)
	default:
		return errors.UnknownError.WithFormat("search for directory anchor %x: %w", syn.Proof.Receipt.Anchor, err)
	}

	// Execute the inner message
	_, err = ctx.callMessageExecutor(batch, syn.Message)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Record the signature (must not fail)
	err = batch.Account(syn.Signature.GetSigner()).
		Transaction(syn.Message.Hash()).
		ValidatorSignatures().
		Add(syn.Signature)
	return errors.InternalError.Wrap(err)
}
