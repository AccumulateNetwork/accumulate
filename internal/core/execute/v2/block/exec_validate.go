// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Validate converts the message to a delivery and validates it. Validate
// returns an error if the message is not a [message.LegacyMessage].
func (x *ExecutorV2) Validate(batch *database.Batch, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	// Make sure every transaction is signed
	err := checkForUnsignedTransactions(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Set up the bundle
	d := new(bundle)
	d.BlockV2 = new(BlockV2)
	d.BlockV2.Executor = (*Executor)(x)
	d.messages = messages
	d.state = orderedMap[[32]byte, *chain.ProcessTransactionState]{cmp: func(u, v [32]byte) int { return bytes.Compare(u[:], v[:]) }}

	// Process each message
	statuses := make([]*protocol.TransactionStatus, len(messages))
	for i, msg := range messages {
		// Prepare the status
		s := new(protocol.TransactionStatus)
		s.TxID = msg.ID()
		statuses[i] = s

		// Validate the message
		ctx := &MessageContext{bundle: d, message: msg}
		err := d.callMessageValidator(batch, ctx)

		// Set the status code
		errCode := errors.Code(err)
		switch {
		case err == nil:
			s.Code = errors.OK
		case errCode.Success():
			s.Code = errCode
		case errCode.IsClientError():
			s.Set(err)
		default:
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return statuses, nil
}

// callMessageValidator finds the executor for the message and calls it.
func (b *bundle) callMessageValidator(batch *database.Batch, ctx *MessageContext) error {
	// Find the appropriate executor
	x, ok := b.Executor.messageExecutors[ctx.Type()]
	if !ok {
		return errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type())
	}

	// Validate the message
	return x.Validate(batch, ctx)
}
