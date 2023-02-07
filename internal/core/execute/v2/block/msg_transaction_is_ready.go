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
	registerSimpleExec[TransactionIsReady](&messageExecutors, internal.MessageTypeTransactionIsReady)
}

// TransactionIsReady queues a transaction for execution.
type TransactionIsReady struct{}

func (TransactionIsReady) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	txn, ok := ctx.message.(*internal.TransactionIsReady)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeTransactionIsReady, ctx.message.Type())
	}

	// Make sure the ID refers to a transaction
	var s messaging.MessageWithTransaction
	err := batch.Message(txn.TxID.Hash()).Main().GetAs(&s)
	switch {
	case errors.Is(err, errors.NotFound):
		return protocol.NewErrorStatus(txn.TxID, err), nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Queue for execution
	ctx.transactionsToProcess.Add(txn.TxID.Hash())

	// The transaction has not yet been processed so don't add its status
	return nil, nil
}
