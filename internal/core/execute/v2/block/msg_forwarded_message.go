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
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return ForwardedMessage{} })
}

// ForwardedMessage marks a message as having been forwarded and processes the
// inner message.
type ForwardedMessage struct{}

func (ForwardedMessage) Type() messaging.MessageType { return internal.MessageTypeForwardedMessage }

func (ForwardedMessage) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	fwd, ok := msg.(*internal.ForwardedMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeForwardedMessage, msg.Type())
	}
	msg = fwd.Message

	// Mark the message as having been forwarded
	b.forwarded.Add(msg.ID().Hash())

	st, err := b.callMessageExecutor(batch, msg)
	err = errors.UnknownError.Wrap(err)
	return st, err
}
