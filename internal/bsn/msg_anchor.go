// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[BlockAnchor](&executors, messaging.MessageTypeBlockAnchor)
}

type BlockAnchor struct{}

func (x BlockAnchor) Validate(batch *ChangeSet, ctx *MessageContext) error {
	msg, _, err := x.check(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Validate the summary
	err = ctx.callMessageValidator(batch, msg.Anchor)
	return errors.UnknownError.Wrap(err)
}

// check checks if the message is garbage or not.
func (x BlockAnchor) check(batch *ChangeSet, ctx *MessageContext) (*messaging.BlockAnchor, *messaging.BlockSummary, error) {
	msg, ok := ctx.message.(*messaging.BlockAnchor)
	if !ok {
		return nil, nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBlockAnchor, ctx.message.Type())
	}

	if msg.Signature == nil {
		return nil, nil, errors.BadRequest.With("missing signature")
	}
	if msg.Anchor == nil {
		return nil, nil, errors.BadRequest.With("missing anchor")
	}
	if msg.Signature.GetTransactionHash() == ([32]byte{}) {
		return nil, nil, errors.BadRequest.With("missing hash")
	}
	if msg.Signature.GetTransactionHash() != msg.Anchor.Hash() {
		return nil, nil, errors.BadRequest.With("wrong hash")
	}

	summary, ok := msg.Anchor.(*messaging.BlockSummary)
	if !ok {
		return nil, nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeBlockSummary, ctx.message.Type())
	}

	h := msg.Anchor.Hash()
	if !msg.Signature.Verify(nil, h[:]) {
		return nil, nil, errors.Unauthenticated.WithFormat("invalid signature")
	}

	// Load the partition's globals
	g := new(core.GlobalValues)
	err := g.Load(protocol.PartitionUrl(summary.Partition), func(accountUrl *url.URL, target interface{}) error {
		return batch.Partition(summary.Partition).Account(accountUrl).Main().GetAs(target)
	})
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load %s globals: %w", summary.Partition, err)
	}

	// Verify the signer is a validator of this partition
	signer := core.AnchorSigner(g, summary.Partition)
	_, _, ok = signer.EntryByKeyHash(msg.Signature.GetPublicKeyHash())
	if !ok {
		return nil, nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", summary.Partition)
	}

	return msg, summary, nil
}

func (x BlockAnchor) Process(batch *ChangeSet, ctx *MessageContext) (err error) {
	batch = batch.Begin()
	defer func() { commitOrDiscard(batch, &err) }()

	// Validate
	msg, summary, err := x.check(batch, ctx)
	switch {
	case err == nil:
		// Ok
	case errors.Code(err).IsClientError():
		ctx.recordErrorStatus(err)
		return nil
	default:
		return errors.UnknownError.Wrap(err)
	}

	// Record the summary if it has not already been recorded
	record := batch.Summary(msg.Anchor.Hash())
	_, err = record.Main().Get()
	switch {
	case err == nil:
		// Already recorded
	case errors.Is(err, errors.NotFound):
		err = record.Main().Put(summary)
		if err != nil {
			return errors.UnknownError.WithFormat("store summary: %w", err)
		}
	default:
		return errors.UnknownError.WithFormat("load summary: %w", err)
	}

	// Record the signature
	err = record.Signatures().Add(msg.Signature)
	if err != nil {
		return errors.UnknownError.WithFormat("store signature: %w", err)
	}

	// Execute the summary (let its executor decide if it's ready)
	err = ctx.callMessageExecutor(batch, summary)
	return errors.UnknownError.Wrap(err)
}
