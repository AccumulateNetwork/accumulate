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
	signature := sig.Signature
	if sig, ok := signature.(*protocol.RemoteSignature); ok {
		signature = sig.Signature
	}
	err = batch.Message2(signature.Hash()).Main().Put(sig)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature: %w", err)
	}
	err = batch.Transaction(signature.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature status: %w", err)
	}

	// Process the transaction if the signature does not fail and is local to
	// the principal
	if !status.Failed() && sig.Signature.RoutingLocation().LocalTo(txn.Header.Principal) {
		_, err := ctx.callMessageExecutor(batch, &messaging.UserTransaction{Transaction: txn})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Don't send out requests unless the transaction succeeds, was not
	// forwarded, and is the initiator
	if status.Failed() ||
		ctx.isWithin(internal.MessageTypeForwardedMessage) ||
		!protocol.SignatureDidInitiate(signature, txn.Header.Initiator[:], nil) {
		return status, nil
	}

	// If the transaction requests additional authorities, send out requests
	for _, auth := range txn.GetAdditionalAuthorities() {
		msg := new(messaging.SignatureRequest)
		msg.Authority = auth
		msg.Cause = sig.ID()
		msg.TxID = txn.ID()
		ctx.didProduce(msg.Authority, msg)
	}

	return status, nil
}
