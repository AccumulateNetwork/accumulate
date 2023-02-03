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
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return UserTransaction{} })
}

// UserTransaction records the transaction but does not execute it. Transactions
// are executed in response to _authority signature_ messages, not user
// transaction messages.
type UserTransaction struct{}

func (UserTransaction) Type() messaging.MessageType { return messaging.MessageTypeUserTransaction }

func (UserTransaction) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	txn, ok := msg.(*messaging.UserTransaction)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeUserTransaction, msg.Type())
	}

	// Ensure the transaction is signed
	var signed bool
	for _, other := range b.messages {
		if fwd, ok := other.(*internal.ForwardedMessage); ok {
			other = fwd.Message
		}
		sig, ok := other.(*messaging.UserSignature)
		if ok && sig.TransactionHash == txn.ID().Hash() {
			signed = true
			break
		}
	}
	if !signed {
		return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("%v is not signed", txn.ID())), nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	isRemote := txn.Transaction.Body.Type() == protocol.TransactionTypeRemote
	record := batch.Transaction(txn.Transaction.GetHash())
	s, err := record.Main().Get()
	switch {
	case errors.Is(err, errors.NotFound) && !isRemote:
		// Store the transaction

	case err != nil:
		// Unknown error or remote transaction with no local copy
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)

	case s.Transaction != nil:
		// It's not a transaction
		return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.With("not a transaction")), nil

	case isRemote || s.Transaction.Equal(txn.Transaction):
		// Transaction has already been recorded
		return nil, nil

	default:
		// This should be impossible
		return nil, errors.InternalError.WithFormat("submitted transaction does not match the locally stored transaction")
	}

	// TODO Can we remove this or do it a better way?
	if txn.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)), nil
	}

	// If we reach this point, Validate should have verified that there is a
	// signer that can be charged for this recording
	err = record.Main().Put(&database.SigOrTxn{Transaction: txn.Transaction})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	// Record when the transaction is received
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if status.Received == 0 {
		status.Received = b.Block.Index
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
