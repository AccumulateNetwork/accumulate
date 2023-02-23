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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[MessageIsReady](&messageExecutors, internal.MessageTypeMessageIsReady)
}

// MessageIsReady executes a message.
type MessageIsReady struct{}

func (MessageIsReady) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, ok := ctx.message.(*internal.MessageIsReady)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeMessageIsReady, ctx.message.Type())
	}

	if msg.TxID == nil {
		return nil, errors.InternalError.With("missing message ID")
	}

	// Load the message
	loaded, err := batch.Message(msg.TxID.Hash()).Main().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return protocol.NewErrorStatus(msg.TxID, err), nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Process the message
	st, err := ctx.callMessageExecutor(batch, loaded)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return st, nil
}
