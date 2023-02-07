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
	registerSimpleExec[UserTransaction](&messageExecutors, messaging.MessageTypeUserTransaction)
}

// UserTransaction records the transaction but does not execute it. Transactions
// are executed in response to _authority signature_ messages, not user
// transaction messages.
type UserTransaction struct{}

func (UserTransaction) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	txn, ok := ctx.message.(*messaging.UserTransaction)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeUserTransaction, ctx.message.Type())
	}

	if txn.Transaction == nil {
		return nil, errors.BadRequest.With("missing transaction")
	}
	if txn.Transaction.Body == nil {
		return nil, errors.BadRequest.With("missing transaction body")
	}

	// TODO Can we remove this or do it a better way?
	if txn.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)), nil
	}

	// Ensure the transaction is signed
	var signed bool
	for _, other := range ctx.messages {
		if fwd, ok := other.(*internal.ForwardedMessage); ok {
			other = fwd.Message
		}
		switch sig := other.(type) {
		case *messaging.UserSignature:
			if sig.TxID.Hash() == txn.ID().Hash() {
				signed = true
				break
			}
		case *messaging.ValidatorSignature:
			if sig.Signature.GetTransactionHash() == txn.ID().Hash() {
				signed = true
				break
			}
		}
	}
	if !signed {
		return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("%v is not signed", txn.ID())), nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	loaded, err := storeTransaction(batch, ctx, txn)
	if err != nil {
		if err, ok := err.(*errors.Error); ok && err.Code.IsClientError() {
			return protocol.NewErrorStatus(txn.ID(), err), nil
		}
		return nil, errors.UnknownError.Wrap(err)
	}

	// If the parent message is synthetic, only allow synthetic transactions.
	// Otherwise, do not allow synthetic transactions.
	if ctx.isWithin(messaging.MessageTypeSynthetic) {
		if !loaded.Body.Type().IsSynthetic() {
			return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("a synthetic message cannot carry a %v", loaded.Body.Type())), nil
		}
	} else {
		if loaded.Body.Type().IsSynthetic() {
			return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("a non-synthetic message cannot carry a %v", loaded.Body.Type())), nil
		}
	}

	// Record when the transaction is received
	record := batch.Transaction(txn.Transaction.GetHash())
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if status.Received == 0 {
		status.Received = ctx.Block.Index
		err = record.Status().Put(status)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// The transaction has not yet been processed so don't add its status
	return nil, nil
}

func storeTransaction(batch *database.Batch, ctx *MessageContext, msg messaging.MessageWithTransaction) (*protocol.Transaction, error) {
	txn := msg.GetTransaction()
	record := batch.Message(txn.ID().Hash())

	// Validate the synthetic transaction header
	if typ := txn.Body.Type(); (typ.IsSynthetic() || typ.IsAnchor()) && !ctx.isWithin(messaging.MessageTypeSequenced) {
		return nil, errors.BadRequest.WithFormat("a %v transaction must be sequenced", typ)
	}

	isRemote := txn.Body.Type() == protocol.TransactionTypeRemote
	s, err := record.Main().Get()
	s2, isTxn := s.(messaging.MessageWithTransaction)
	switch {
	case errors.Is(err, errors.NotFound) && !isRemote:
		// Store the transaction

	case err != nil:
		// Unknown error or remote transaction with no local copy
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)

	case !isTxn:
		// It's not a transaction
		return nil, errors.BadRequest.With("not a transaction")

	case isRemote || s2.GetTransaction().Equal(txn):
		// Transaction has already been recorded
		return s2.GetTransaction(), nil

	default:
		// This should be impossible
		return nil, errors.InternalError.WithFormat("submitted transaction does not match the locally stored transaction")
	}

	// If we reach this point, Validate should have verified that there is a
	// signer that can be charged for this recording
	err = record.Main().Put(msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	return txn, nil
}
