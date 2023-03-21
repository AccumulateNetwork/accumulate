// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SignatureMessage](&messageExecutors, messaging.MessageTypeSignature)
}

// SignatureMessage executes the signature, queuing the transaction for processing
// when appropriate.
type SignatureMessage struct{}

func (x SignatureMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	sig, txn, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	status, err := ctx.callSignatureValidator(batch, ctx.sigWith(sig.Signature, txn))
	return status, errors.UnknownError.Wrap(err)
}

func (SignatureMessage) check(batch *database.Batch, ctx *MessageContext) (*messaging.SignatureMessage, *protocol.Transaction, error) {
	sig, ok := ctx.message.(*messaging.SignatureMessage)
	if !ok {
		return nil, nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSignature, ctx.message.Type())
	}

	// Basic validation
	if sig.Signature == nil {
		return nil, nil, errors.BadRequest.With("missing signature")
	}
	if sig.TxID == nil {
		return nil, nil, errors.BadRequest.With("missing transaction ID")
	}

	// TODO FIXME If we're within MessageIsReady, presumably this validation has
	// already been done, so it should be safe to skip it. But honestly, this is
	// questionable logic and indicates a larger problem.
	if !ctx.isWithin(internal.MessageTypeMessageIsReady) {
		// Verify the bundle contains the transaction
		var hasTxn bool
		for _, msg := range ctx.messages {
			txn, ok := msg.(*messaging.TransactionMessage)
			if !ok {
				continue
			}
			if txn.Hash() == sig.TxID.Hash() {
				hasTxn = true
				break
			}
		}
		if !hasTxn {
			return nil, nil, errors.BadRequest.With("cannot process a signature without its transaction")
		}
	}

	// Only allow authority signatures within a synthetic message and don't
	// allow them outside of one
	if ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		if sig.Signature.Type() != protocol.SignatureTypeAuthority {
			return nil, nil, errors.BadRequest.WithFormat("a synthetic message cannot carry a %v signature", sig.Signature.Type())
		}
	} else {
		if sig.Signature.Type() == protocol.SignatureTypeAuthority {
			return nil, nil, errors.BadRequest.WithFormat("a non-synthetic message cannot carry a %v signature", sig.Signature.Type())
		}
	}

	// Load and check the transaction
	txn, err := ctx.getTransaction(batch, sig.TxID.Hash())
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}
	if txn.Header.Principal == nil {
		return nil, nil, errors.BadRequest.WithFormat("invalid transaction: missing principal")
	}
	if txn.Body == nil {
		return nil, nil, errors.BadRequest.WithFormat("invalid transaction: missing body")
	}
	if !txn.Body.Type().IsUser() {
		return nil, nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", txn.Body.Type(), sig.Type())
	}

	// Use the full transaction ID (since normalization uses unknown.acme)
	sig.TxID = txn.ID()

	return sig, txn, nil
}

func (x SignatureMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Make sure the block is recorded
	ctx.Block.State.MergeSignature(&ProcessSignatureState{})

	// Process the message
	sig, txn, err := x.check(batch, ctx)
	if err == nil {
		_, err = ctx.callSignatureExecutor(batch, ctx.sigWith(sig.Signature, txn))
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}
