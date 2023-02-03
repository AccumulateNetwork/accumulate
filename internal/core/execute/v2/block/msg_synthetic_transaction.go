// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return SyntheticTransaction{} })
}

// SyntheticTransaction records the synthetic transaction but does not execute
// it.
type SyntheticTransaction struct{}

func (SyntheticTransaction) Type() messaging.MessageType {
	return messaging.MessageTypeSyntheticTransaction
}

func (SyntheticTransaction) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	txn, ok := msg.(*messaging.SyntheticTransaction)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSyntheticTransaction, msg.Type())
	}

	// Basic validation
	if txn.Transaction == nil {
		return nil, errors.BadRequest.With("missing transaction")
	}
	if txn.Transaction.Body == nil {
		return nil, errors.BadRequest.With("missing transaction body")
	}
	if txn.Proof != nil {
		if txn.Proof.Receipt == nil {
			return nil, errors.BadRequest.With("missing proof receipt")
		}
		if txn.Proof.Anchor == nil || txn.Proof.Anchor.Account == nil {
			return nil, errors.BadRequest.With("missing proof metadata")
		}
	}

	// TODO Can we remove this or do it a better way?
	if txn.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)), nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the status
	record := batch.Transaction(txn.Transaction.GetHash())
	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Ensure the transaction is signed
	if len(status.Signers) == 0 {
		var signed bool
		for _, other := range b.messages {
			if fwd, ok := other.(*internal.ForwardedMessage); ok {
				other = fwd.Message
			}
			sig, ok := other.(*messaging.ValidatorSignature)
			if ok && sig.Signature.GetTransactionHash() == txn.ID().Hash() {
				signed = true
				break
			}
		}
		if !signed {
			return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.WithFormat("%v is not signed", txn.ID())), nil
		}
	}

	// Store the transaction (or load it if its remote)
	loaded, err := storeTransaction(batch, txn)
	if err != nil {
		if err, ok := err.(*errors.Error); ok && err.Code.IsClientError() {
			return protocol.NewErrorStatus(txn.ID(), err), nil
		}
		return nil, errors.UnknownError.Wrap(err)
	}

	// Only allow synthetic transactions
	if !loaded.Body.Type().IsSynthetic() {
		return nil, errors.BadRequest.WithFormat("transaction type %v is not compatible with message type %v", loaded.Body.Type(), msg.Type())
	}

	// Record when the transaction was first received
	if status.Received == 0 {
		status.Received = b.Block.Index
	}

	// Record the proof (if provided)
	if txn.Proof != nil {
		if txn.Proof.Anchor.Account.Equal(protocol.DnUrl()) {
			status.GotDirectoryReceipt = true
		}

		switch {
		case status.Proof != nil && bytes.Equal(status.Proof.Anchor, txn.Proof.Receipt.Start):
			// The incoming proof extends the one we have
			status.Proof, err = status.Proof.Combine(txn.Proof.Receipt)
			if err != nil {
				return nil, errors.Unauthorized.WithFormat("combine receipts: %w", err)
			}

		case !bytes.Equal(txn.Transaction.GetHash(), txn.Proof.Receipt.Start):
			return nil, errors.Unauthorized.WithFormat("receipt does not match transaction")

			// Else the incoming proof starts from the transaction hash

		case status.Proof == nil:
			// We have no proof yet
			status.Proof = txn.Proof.Receipt

		case status.Proof.Contains(txn.Proof.Receipt):
			// We already have the proof

		case txn.Proof.Receipt.Contains(status.Proof):
			// The incoming proof contains and extends the one we have
			status.Proof = txn.Proof.Receipt

		default:
			return protocol.NewErrorStatus(txn.ID(), errors.BadRequest.With("incoming receipt is incompatible with existing receipt")), nil
		}
	}

	err = record.Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Queue for execution
	b.transactionsToProcess.Add(txn.ID().Hash())

	// The transaction has not yet been processed so don't add its status
	return nil, nil
}
