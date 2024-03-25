// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerConditionalExec[MakeMajorBlock](&messageExecutors,
		func(ctx *MessageContext) bool { return ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() },
		messaging.MessageTypeMakeMajorBlock)
}

// MakeMajorBlock lists a transaction as pending on an authority.
type MakeMajorBlock struct{}

func (x MakeMajorBlock) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	_, err := x.check(ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (MakeMajorBlock) check(ctx *MessageContext) (*messaging.MakeMajorBlock, error) {
	req, ok := ctx.message.(*messaging.MakeMajorBlock)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeMakeMajorBlock, ctx.message.Type())
	}

	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
		return nil, errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", req.Type())
	}

	return req, nil
}

func (x MakeMajorBlock) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Process the message
	req, err := x.check(ctx)
	if err == nil {
		err = x.process(ctx, req)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x MakeMajorBlock) process(ctx *MessageContext, req *messaging.MakeMajorBlock) error {
	if ctx.Executor.Describe.NetworkType == protocol.PartitionTypeDirectory {
		return nil
	}

	state := new(chain.ProcessTransactionState)
	state.MakeMajorBlock = req.MajorBlockIndex
	state.MakeMajorBlockTime = req.MajorBlockTime
	ctx.state.Set(ctx.message.Hash(), state)
	return nil
}
