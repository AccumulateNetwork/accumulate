// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slices"
)

func init() {
	registerConditionalExec[DidUpdateExecutorVersion](&messageExecutors,
		func(ctx *MessageContext) bool { return ctx.GetActiveGlobals().ExecutorVersion.V2VandenbergEnabled() },
		messaging.MessageTypeDidUpdateExecutorVersion)
}

// DidUpdateExecutorVersion constructs a transaction for the network update and queues it
// for processing.
type DidUpdateExecutorVersion struct{}

func (x DidUpdateExecutorVersion) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, ok := ctx.message.(*messaging.DidUpdateExecutorVersion)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeDidUpdateExecutorVersion, ctx.message.Type())
	}

	err := x.check(ctx, msg)
	return nil, err
}

func (DidUpdateExecutorVersion) check(ctx *MessageContext, msg *messaging.DidUpdateExecutorVersion) error {
	// Must be synthetic
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady, internal.MessageTypePseudoSynthetic) {
		return errors.BadRequest.WithFormat("cannot execute %v outside of a synthetic message", msg.Type())
	}

	ok := slices.ContainsFunc(ctx.GetActiveGlobals().BvnNames(), func(name string) bool {
		return strings.EqualFold(name, msg.Partition)
	})
	if !ok {
		return errors.BadRequest.WithFormat("%q is not the name of a BVN", msg.Partition)
	}

	return nil
}

func (x DidUpdateExecutorVersion) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	msg, ok := ctx.message.(*messaging.DidUpdateExecutorVersion)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeDidUpdateExecutorVersion, ctx.message.Type())
	}

	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message
	err = x.check(ctx, msg)
	if err == nil {
		err = x.process(batch, ctx, msg)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !status.Code.Success() {
		return status, nil
	}
	return status, errors.UnknownError.Wrap(err)
}

func (DidUpdateExecutorVersion) process(batch *database.Batch, ctx *MessageContext, msg *messaging.DidUpdateExecutorVersion) error {
	var ledger *protocol.SystemLedger
	err := batch.Account(ctx.Executor.Describe.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	// Ensure there's an entry for each BVN (SetBvnExecutorVersion will not
	// lower the version number)
	for _, bvn := range ctx.GetActiveGlobals().BvnNames() {
		ledger.SetBvnExecutorVersion(bvn, 0)
	}

	// Update the specified entry
	ledger.SetBvnExecutorVersion(msg.Partition, msg.Version)

	// Update the ledger
	err = batch.Account(ctx.Executor.Describe.Ledger()).Main().Put(ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("store system ledger: %w", err)
	}

	// Update globals
	ctx.Executor.globals.Pending.BvnExecutorVersions = ledger.BvnExecutorVersions
	return nil
}
