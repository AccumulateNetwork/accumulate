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

type bundle struct {
	*Block

	statuses   []*protocol.TransactionStatus
	additional []messaging.Message
}

func (b *Block) Process(messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	var statuses []*protocol.TransactionStatus

	for len(messages) > 0 {
		d := &bundle{Block: b}

		// Process each message
		for _, msg := range messages {
			ctx := &MessageContext{bundle: d, message: msg}
			err := d.callMessageExecutor(b.batch, ctx)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}

			d.statuses = append(d.statuses, ctx.statuses...)
			d.additional = append(d.additional, ctx.additional...)
			b.stats.Processed++
		}

		statuses = append(statuses, d.statuses...)
		messages = d.additional
	}

	return statuses, nil
}

// callMessageExecutor finds the executor for the message and calls it.
func (b *bundle) callMessageExecutor(batch *ChangeSet, ctx *MessageContext) error {
	// Find the appropriate executor
	x, ok := b.executor.executors[ctx.Type()]
	if !ok {
		ctx.recordErrorStatus(errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type()))
		return nil
	}

	// Process the message
	err := x.Process(batch, ctx)
	return errors.UnknownError.Wrap(err)
}
