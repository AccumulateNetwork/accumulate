// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// beginValidation begins the read-only batch a CheckTx validates against.
// Validation wants the LATEST committed state, not a snapshot of the state
// when it began, so on a store that keeps isolation by remembering
// pre-images (BlockchainDB) the batch pins no version: every CheckTx used
// to register a reader, so one was almost always open while a block
// committed and every commit paid a store read per dynamic entry for an
// isolation nobody needed (#4237, database spec "Windowed stores"). On any
// other store this is an ordinary read batch.
func (x *Executor) beginValidation() *database.Batch {
	if u, ok := x.db.(interface{ Unisolated() *database.Database }); ok {
		return u.Unisolated().Begin(false)
	}
	return x.db.Begin(false)
}

// Validate converts the message to a delivery and validates it. Validate
// returns an error if the message is not a [message.LegacyMessage].
func (x *Executor) Validate(envelope *messaging.Envelope, _ bool) ([]*protocol.TransactionStatus, error) {
	batch := x.beginValidation()
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

	// Set up the bundle. Validation reads staging the way a block does — a
	// placeholder resolves against a held anchor — and adds nothing to it.
	d := new(bundle)
	d.Block = new(Block)
	d.Block.Executor = x
	d.Block.staging = x.staging().Begin()
	defer d.Block.staging.Discard()
	d.messages = messages

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
	x, ok := getExecutor(b.Executor.messageExecutors, ctx)
	if !ok {
		return nil, errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type())
	}

	// Validate the message
	return x.Validate(batch, ctx)
}

// callSignatureValidator finds the executor for the signature and calls it.
func (b *bundle) callSignatureValidator(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	// Find the appropriate executor
	x, ok := getExecutor(b.Executor.signatureExecutors, ctx)
	if !ok {
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("unsupported signature type %v", ctx.Type())), nil
	}

	// Validate the message
	st, err := x.Validate(batch, ctx)
	err = errors.UnknownError.Wrap(err)
	return st, err
}
