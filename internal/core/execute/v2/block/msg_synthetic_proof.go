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

func init() {
	// Conditional, because what a node is willing to accept is consensus
	// critical: a node that took a proof-carrying envelope while another
	// rejected it would disagree about state. Nothing emits one before Kourou.
	registerConditionalExec[SyntheticProof](&messageExecutors,
		func(ctx *MessageContext) bool {
			return ctx.GetActiveGlobals().ExecutorVersion.V2KourouEnabled()
		},
		messaging.MessageTypeSyntheticProof)
}

// SyntheticProof carries one collection proof covering every synthetic message
// in the same envelope (#4090).
//
// It has NO effect of its own. It is not recorded, produces nothing, and
// authorizes nothing by itself — the synthetic messages travelling with it find
// it in the bundle and verify themselves against it. Executing it is therefore a
// no-op beyond validating that it is well formed, which is the point: the proof
// is data the siblings consume, not an instruction.
//
// It still needs an executor. Without one the bundle fails on an unhandled
// message type and the whole envelope is rejected, taking the synthetics with it.
type SyntheticProof struct{}

func (x SyntheticProof) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	_, err := x.check(ctx)
	return nil, errors.UnknownError.Wrap(err)
}

func (x SyntheticProof) check(ctx *MessageContext) (*messaging.SyntheticProof, error) {
	msg, ok := ctx.message.(*messaging.SyntheticProof)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v",
			messaging.MessageTypeSyntheticProof, ctx.message.Type())
	}
	if msg.Proof == nil {
		return nil, errors.BadRequest.With("missing proof")
	}
	if msg.Proof.ReceiptList == nil {
		return nil, errors.BadRequest.With("a synthetic proof must carry a receipt list")
	}
	if msg.Proof.Receipt != nil {
		return nil, errors.BadRequest.With("a synthetic proof must carry a receipt list, not an individual receipt")
	}
	// Bound the work before doing any: an unbounded list is an invitation to make
	// a validator allocate and hash arbitrarily much.
	if len(msg.Proof.ReceiptList.Elements) > protocol.MaxReceiptListElements {
		return nil, errors.BadRequest.WithFormat("collection proof carries %d elements, limit is %d",
			len(msg.Proof.ReceiptList.Elements), protocol.MaxReceiptListElements)
	}
	if !msg.Proof.ReceiptList.Validate(nil) {
		return nil, errors.BadRequest.With("proof is invalid")
	}
	return msg, nil
}

func (x SyntheticProof) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// Validate the shape so a malformed proof fails here rather than confusing
	// each sibling separately, then do nothing. The siblings resolve against it.
	_, err := x.check(ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	status := new(protocol.TransactionStatus)
	status.TxID = ctx.message.ID()
	status.Code = errors.Delivered
	return status, nil
}
