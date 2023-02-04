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
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return SyntheticMessage{} })
}

// SyntheticMessage records the synthetic transaction but does not execute
// it.
type SyntheticMessage struct{}

func (SyntheticMessage) Type() messaging.MessageType {
	return messaging.MessageTypeSynthetic
}

func (SyntheticMessage) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	syn, ok := ctx.message.(*messaging.SyntheticMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSynthetic, ctx.message.Type())
	}

	// Basic validation
	if syn.Message == nil {
		return nil, errors.BadRequest.With("missing message")
	}
	if syn.Proof != nil {
		if syn.Proof.Receipt == nil {
			return nil, errors.BadRequest.With("missing proof receipt")
		}
		if syn.Proof.Anchor == nil || syn.Proof.Anchor.Account == nil {
			return nil, errors.BadRequest.With("missing proof metadata")
		}
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the status
	h := syn.ID().Hash()
	status, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	// Record when the transaction was first received
	if status.Received == 0 {
		status.Received = ctx.Block.Index
	}

	// Record the proof (if provided)
	if syn.Proof != nil {
		if syn.Proof.Anchor.Account.Equal(protocol.DnUrl()) {
			status.GotDirectoryReceipt = true
		}

		switch {
		case status.Proof != nil && bytes.Equal(status.Proof.Anchor, syn.Proof.Receipt.Start):
			// The incoming proof extends the one we have
			status.Proof, err = status.Proof.Combine(syn.Proof.Receipt)
			if err != nil {
				return protocol.NewErrorStatus(syn.ID(), errors.Unauthorized.WithFormat("combine receipts: %w", err)), nil
			}

		case !bytes.Equal(h[:], syn.Proof.Receipt.Start):
			return protocol.NewErrorStatus(syn.ID(), errors.Unauthorized.WithFormat("receipt does not match transaction")), nil

			// Else the incoming proof starts from the transaction hash

		case status.Proof == nil:
			// We have no proof yet
			status.Proof = syn.Proof.Receipt

		case status.Proof.Contains(syn.Proof.Receipt):
			// We already have the proof

		case syn.Proof.Receipt.Contains(status.Proof):
			// The incoming proof contains and extends the one we have
			status.Proof = syn.Proof.Receipt

		default:
			return protocol.NewErrorStatus(syn.ID(), errors.BadRequest.With("incoming receipt is incompatible with existing receipt")), nil
		}
	}

	err = batch.Transaction(h[:]).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	var shouldQueue bool
	switch syn.Message.Type() {
	case messaging.MessageTypeUserTransaction:
		// Allowed
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
