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
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return NetworkUpdate{} })
}

// NetworkUpdate constructs a transaction and signature for the network update,
// stores the transaction, and executes the signature (which queues the
// transaction for processing).
type NetworkUpdate struct{}

func (NetworkUpdate) Type() messaging.MessageType { return internal.MessageTypeNetworkUpdate }

func (NetworkUpdate) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	update, ok := msg.(*internal.NetworkUpdate)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeNetworkUpdate, msg.Type())
	}

	sig := new(protocol.InternalSignature)
	sig.Cause = update.Cause
	txn := new(protocol.Transaction)
	txn.Header.Principal = update.Account
	txn.Header.Initiator = *(*[32]byte)(sig.Metadata().Hash())
	txn.Body = update.Body
	sig.TransactionHash = *(*[32]byte)(txn.GetHash())

	b.internal.Add(txn.ID().Hash())
	b.internal.Add(*(*[32]byte)(sig.Hash()))

	batch = batch.Begin(true)
	defer batch.Discard()

	// Store the transaction
	err := batch.Transaction(sig.TransactionHash[:]).Main().Put(&database.SigOrTxn{Transaction: txn})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	// Process the signature
	st, err := b.callMessageExecutor(batch, &messaging.UserSignature{
		Signature:       sig,
		TransactionHash: txn.ID().Hash(),
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return st, nil
}
