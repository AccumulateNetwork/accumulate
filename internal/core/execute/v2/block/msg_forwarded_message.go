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
	registerSimpleExec[ForwardedMessage](&messageExecutors, internal.MessageTypeForwardedMessage)
}

// ForwardedMessage marks a message as having been forwarded and processes the
// inner message.
type ForwardedMessage struct{}

func (ForwardedMessage) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	fwd, ok := ctx.message.(*internal.ForwardedMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeForwardedMessage, ctx.message.Type())
	}
	msg := fwd.Message

	// Mark the message as having been forwarded
	ctx.forwarded.Add(msg.ID().Hash())

	st, err := ctx.callMessageExecutor(batch, ctx.childWith(msg))
	err = errors.UnknownError.Wrap(err)
	return st, err
}
