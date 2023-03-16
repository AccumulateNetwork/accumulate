// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func init() {
	registerSimpleExec[BlockSummary](&executors, messaging.MessageTypeBlockSummary)
}

type BlockSummary struct{}

func (x BlockSummary) Validate(batch *ChangeSet, ctx *MessageContext) error {
	_, err := x.check(batch, ctx)
	return err
}

func (BlockSummary) check(batch *ChangeSet, ctx *MessageContext) (*messaging.BlockSummary, error) {
	msg, ok := ctx.message.(*messaging.BlockSummary)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBlockAnchor, ctx.message.Type())
	}

	if msg.Partition == "" {
		return nil, errors.BadRequest.With("missing partition")
	}
	if msg.Index == 0 {
		return nil, errors.BadRequest.With("missing index")
	}

	return msg, nil
}

func (x BlockSummary) Process(batch *ChangeSet, ctx *MessageContext) (err error) {
	batch = batch.Begin()
	defer func() { commitOrDiscard(batch, &err) }()

	_, err = x.check(batch, ctx)
	return err
}
