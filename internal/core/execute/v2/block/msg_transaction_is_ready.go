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
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return TransactionIsReady{} })
}

// TransactionIsReady queues a transaction for execution.
type TransactionIsReady struct{}

func (TransactionIsReady) Type() messaging.MessageType {
	return internal.MessageTypeTransactionIsReady
}

func (TransactionIsReady) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	txn, ok := msg.(*internal.TransactionIsReady)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeTransactionIsReady, msg.Type())
	}

	// Make sure the ID refers to a transaction
	h := txn.TxID.Hash()
	s, err := batch.Transaction(h[:]).Main().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return protocol.NewErrorStatus(txn.TxID, err), nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	case s.Transaction == nil:
		return protocol.NewErrorStatus(txn.TxID, errors.BadRequest.WithFormat("%v is not a transaction", txn.ID())), nil
	}

	// Queue for execution
	b.transactionsToProcess.Add(txn.TxID.Hash())

	// The transaction has not yet been processed so don't add its status
	return nil, nil
}
