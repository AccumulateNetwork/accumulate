// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SyntheticMessage](&messageExecutors, messaging.MessageTypeSynthetic)
}

// SyntheticMessage records the synthetic transaction but does not execute
// it.
type SyntheticMessage struct{}

func (SyntheticMessage) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	syn, ok := ctx.message.(*messaging.SyntheticMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSynthetic, ctx.message.Type())
	}

	// Basic validation
	if syn.Message == nil {
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.With("missing message")), nil
	}
	if syn.Proof == nil {
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.With("missing proof")), nil
	}
	if syn.Proof.Receipt == nil {
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.With("missing proof receipt")), nil
	}
	if syn.Proof.Anchor == nil || syn.Proof.Anchor.Account == nil {
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.With("missing proof metadata")), nil
	}

	// Verify the proof starts with the transaction hash
	h := syn.ID().Hash()
	if !bytes.Equal(h[:], syn.Proof.Receipt.Start) {
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.WithFormat("invalid proof start: expected %x, got %x", h, syn.Proof.Receipt.Start)), nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Verify the proof ends with a DN anchor
	_, err := batch.Account(ctx.Executor.Describe.AnchorPool()).AnchorChain(protocol.Directory).Root().IndexOf(syn.Proof.Receipt.Anchor)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.WithFormat("invalid proof anchor: %x is not a known directory anchor", syn.Proof.Receipt.Anchor)), nil
	default:
		return nil, errors.UnknownError.WithFormat("search for directory anchor %x: %w", syn.Proof.Receipt.Anchor, err)
	}

	// Record when the transaction was first received
	status, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Received == 0 {
		status.Received = ctx.Block.Index
		err = batch.Transaction(h[:]).Status().Put(status)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	var shouldQueue bool
	switch syn.Message.Type() {
	case messaging.MessageTypeUserTransaction:
		// Allowed, queue for execution
		shouldQueue = true

	default:
		return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.WithFormat("a synthetic message cannot carry a %v", syn.Message.Type())), nil
	}

	st, err := ctx.callMessageExecutor(batch, ctx.childWith(syn.Message))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if st != nil && st.Failed() {
		return st, nil
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if shouldQueue {
		// Queue for execution
		ctx.transactionsToProcess.Add(syn.ID().Hash())
	}

	return st, nil
}
