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

	if sig, ok := sig.Signature.(protocol.KeySignature); ok && sig.GetSigner().Equal(protocol.DnUrl().JoinPath(protocol.Network)) {
		return nil, errors.BadRequest.With("anchor signatures must be submitted via a ValidatorSignature message")
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the transaction
	txn, err := batch.Transaction(sig.TransactionHash[:]).Main().Get()
	switch {
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	case txn.Transaction == nil:
		return protocol.NewErrorStatus(sig.ID(), errors.BadRequest.WithFormat("%x is not a transaction", sig.TransactionHash)), nil
	}

	if typ := txn.Transaction.Body.Type(); typ.IsSynthetic() || typ.IsAnchor() {
		return nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", typ, msg.Type())
	}

	// Process the transaction if it is synthetic or system, or the signature is
	// internal, or the signature is local to the principal
	signature, transaction := sig.Signature, txn.Transaction
	if !transaction.Body.Type().IsUser() ||
		signature.Type() == protocol.SignatureTypeInternal ||
		signature.RoutingLocation().LocalTo(transaction.Header.Principal) {
		b.transactionsToProcess.Add(transaction.ID().Hash())
	}

	status := new(protocol.TransactionStatus)
	status.TxID = sig.ID()
	status.Received = b.Block.Index

	s, err := b.Executor.ProcessSignature(batch, &chain.Delivery{
		Transaction: transaction,
		Internal:    b.internal.Has(sig.ID().Hash()),
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
	err = batch.Transaction(signature.Hash()).Main().Put(&database.SigOrTxn{Signature: signature, Txid: transaction.ID()})
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
