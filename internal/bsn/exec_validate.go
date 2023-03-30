// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (x *Executor) Validate(messages []messaging.Message, recheck bool) ([]*protocol.TransactionStatus, error) {
	b := new(Block)
	b.executor = x
	b.batch = NewChangeSet(x.store, x.logger)
	defer b.batch.Discard()

	d := new(bundle)
	d.Block = b

	// Validate each message
	var statuses []*protocol.TransactionStatus
	for _, msg := range messages {
		ctx := &MessageContext{bundle: d, message: msg}
		err := d.callMessageValidator(b.batch, ctx)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		statuses = append(statuses, ctx.statuses...)
	}
	return statuses, nil
}

// callMessageValidator finds the validator for the message and calls it.
func (b *bundle) callMessageValidator(batch *ChangeSet, ctx *MessageContext) error {
	// Find the appropriate Validator
	x, ok := b.executor.executors[ctx.Type()]
	if !ok {
		ctx.recordErrorStatus(errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type()))
		return nil
	}

	// Validate the message
	err := x.Validate(batch, ctx)
	return errors.UnknownError.Wrap(err)
}
