// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[UserSignature](&messageExecutors, messaging.MessageTypeUserSignature)
}

// UserSignature executes the signature, queuing the transaction for processing
// when appropriate.
type UserSignature struct{}

func (x UserSignature) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	sig, txn, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	status, err := ctx.callSignatureValidator(batch, ctx.sigWith(sig.Signature, txn))
	return status, errors.UnknownError.Wrap(err)
}

func (UserSignature) check(batch *database.Batch, ctx *MessageContext) (*messaging.UserSignature, *protocol.Transaction, error) {
	sig, ok := ctx.message.(*messaging.UserSignature)
	if !ok {
		return nil, nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeUserSignature, ctx.message.Type())
	}

	// Basic validation
	if sig.Signature == nil {
		return nil, nil, errors.BadRequest.With("missing signature")
	}
	if sig.TxID == nil {
		return nil, nil, errors.BadRequest.With("missing transaction ID")
	}

	// Verify the bundle contains the transaction
	var hasTxn bool
	for _, msg := range ctx.messages {
		txn, ok := msg.(*messaging.UserTransaction)
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

func (x UserSignature) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	batch = batch.Begin(true)
	defer batch.Discard()

	// Check if the message has already been processed
	status, err := batch.Transaction2(ctx.message.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Delivered() {
		return status, nil
	}

	// Record when the message was first received
	if status.Received == 0 {
		status.Received = ctx.Block.Index
		status.TxID = ctx.message.ID()
	}

	// Check the message
	sig, txn, err := x.check(batch, ctx)
	switch {
	case err == nil:
		// Ok

	case errors.Code(err).IsClientError():
		status.Set(err)

	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process the signature
	if !status.Failed() {
		status, err = ctx.callSignatureExecutor(batch, ctx.sigWith(sig.Signature, txn))
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Always record the signature and status
	err = batch.Message(sig.Hash()).Main().Put(sig)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature: %w", err)
	}
	err = batch.Transaction2(sig.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature status: %w", err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}
