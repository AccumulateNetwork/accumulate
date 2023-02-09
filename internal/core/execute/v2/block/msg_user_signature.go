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

func (UserSignature) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	sig, ok := ctx.message.(*messaging.UserSignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeUserSignature, ctx.message.Type())
	}
	if sig.Signature.Type().IsSystem() {
		return nil, errors.BadRequest.WithFormat("cannot submit a %v signature with a %v message", sig.Signature.Type(), sig.Type())
	}

	// Only allow authority signatures within a synthetic message and don't
	// allow them outside of one
	if ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		if sig.Signature.Type() != protocol.SignatureTypeAuthority {
			return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("a synthetic message cannot carry a %v signature", sig.Signature.Type())), nil
		}
	} else {
		if sig.Signature.Type() == protocol.SignatureTypeAuthority {
			return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("a non-synthetic message cannot carry a %v signature", sig.Signature.Type())), nil
		}
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the transaction
	txn, err := ctx.getTransaction(batch, sig.TxID.Hash())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Use the full transaction ID (since normalization uses unknown.acme)
	sig.TxID = txn.ID()

	if !txn.Body.Type().IsUser() {
		return nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", txn.Body.Type(), sig.Type())
	}

	// Process the signature
	status, err := ctx.callSignatureExecutor(batch, ctx.sigWith(sig.Signature, txn))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Always record the signature and status
	err = batch.Message2(sig.Signature.Hash()).Main().Put(sig)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature: %w", err)
	}
	err = batch.Transaction(sig.Signature.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature status: %w", err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}
