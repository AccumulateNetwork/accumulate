// Copyright 2024 The Accumulate Authors
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
func (x *Executor) Validate(envelope *messaging.Envelope, _ bool) ([]*protocol.TransactionStatus, error) {
	batch := x.db.Begin(false)
	defer batch.Discard()

	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Make sure every transaction is signed
	err = x.checkForUnsignedTransactions(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Set up the bundle
	d := new(bundle)
	d.Block = new(Block)
	d.Block.Executor = x
	d.messages = messages
	d.state = orderedMap[[32]byte, *chain.ProcessTransactionState]{cmp: func(u, v [32]byte) int { return bytes.Compare(u[:], v[:]) }}

	// Process each message
	statuses := make([]*protocol.TransactionStatus, len(messages))
	for i, msg := range messages {
		// Validate the message
		ctx := &MessageContext{bundle: d, message: msg}
		s, err := d.callMessageValidator(batch, ctx)
		if s == nil {
			s = new(protocol.TransactionStatus)
			s.TxID = msg.ID()
		}
		statuses[i] = s

		// Set the status code
		errCode := errors.Code(err)
		switch {
		case err == nil:
			if s.Code == 0 {
				s.Code = errors.OK
			}
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
func (b *bundle) callMessageValidator(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// Find the appropriate executor
	x, ok := b.Executor.messageExecutors[ctx.Type()]
	if !ok {
		return nil, errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type())
	}

	// Validate the message
	return x.Validate(batch, ctx)
}

// callSignatureValidator finds the executor for the signature and calls it.
func (b *bundle) callSignatureValidator(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	// Find the appropriate executor
	x, ok := b.Executor.signatureExecutors[ctx.Type()]
	if !ok {
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("unsupported signature type %v", ctx.Type())), nil
	}

	// Validate the message
	st, err := x.Validate(batch, ctx)
	err = errors.UnknownError.Wrap(err)
	return st, err
}
