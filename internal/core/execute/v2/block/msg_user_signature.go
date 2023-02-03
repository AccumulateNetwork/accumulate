// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return UserSignature{} })
}

// UserSignature executes the signature, queuing the transaction for processing
// when appropriate.
type UserSignature struct{}

func (UserSignature) Type() messaging.MessageType { return messaging.MessageTypeUserSignature }

func (UserSignature) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	sig, ok := msg.(*messaging.UserSignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeUserSignature, msg.Type())
	}
	if sig.Signature.Type().IsSystem() {
		return nil, errors.BadRequest.WithFormat("cannot submit a %v signature with a %v message", sig.Signature.Type(), msg.Type())
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the transaction
	var txn messaging.MessageWithTransaction
	err := batch.Message(sig.TxID.Hash()).Main().GetAs(&txn)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Use the full transaction ID (since normalization uses unknown.acme)
	sig.TxID = txn.ID()

	if !txn.GetTransaction().Body.Type().IsUser() {
		return nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", txn.GetTransaction().Body.Type(), msg.Type())
	}

	// Process the transaction if it is synthetic or system, or the signature is
	// internal, or the signature is local to the principal
	signature, transaction := sig.Signature, txn.GetTransaction()
	if signature.RoutingLocation().LocalTo(transaction.Header.Principal) {
		b.transactionsToProcess.Add(transaction.ID().Hash())
	}

	status := new(protocol.TransactionStatus)
	status.TxID = sig.ID()
	status.Received = b.Block.Index

	s, err := b.Executor.processSignature2(batch, &chain.Delivery{
		Transaction: transaction,
		Forwarded:   b.forwarded.Has(sig.ID().Hash()),
	}, signature)
	b.Block.State.MergeSignature(s)
	if err == nil {
		status.Code = errors.Delivered
	} else {
		status.Set(err)
	}

	// Always record the signature and status
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

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return status, nil
}
