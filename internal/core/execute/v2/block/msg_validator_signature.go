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
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return ValidatorSignature{} })
}

// ValidatorSignature executes the signature, queuing the transaction for processing
// when appropriate.
type ValidatorSignature struct{}

func (ValidatorSignature) Type() messaging.MessageType {
	return messaging.MessageTypeValidatorSignature
}

func (ValidatorSignature) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	sig, ok := msg.(*messaging.ValidatorSignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeValidatorSignature, msg.Type())
	}
	if sig.Signature == nil {
		return protocol.NewErrorStatus(sig.ID(), errors.BadRequest.With("missing signature")), nil
	}
	if sig.Source == nil {
		return protocol.NewErrorStatus(sig.ID(), errors.BadRequest.With("missing source")), nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the transaction
	h := sig.Signature.GetTransactionHash()
	txn, err := batch.Transaction(h[:]).Main().Get()
	switch {
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	case txn.Transaction == nil:
		return protocol.NewErrorStatus(sig.ID(), errors.BadRequest.WithFormat("%x is not a transaction", h)), nil
	}

	// A validator signature message is only allowed for synthetic and anchor
	// transactions
	if typ := txn.Transaction.Body.Type(); !typ.IsSynthetic() && !typ.IsAnchor() {
		return protocol.NewErrorStatus(sig.ID(), errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", typ, msg.Type())), nil
	}

	status := new(protocol.TransactionStatus)
	status.TxID = sig.ID()
	status.Received = b.Block.Index

	signature, transaction := sig.Signature, txn.Transaction
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

	// Queue for execution
	b.transactionsToProcess.Add(transaction.ID().Hash())

	return status, nil
}
