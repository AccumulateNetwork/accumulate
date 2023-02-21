// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SequencedMessage](&messageExecutors, messaging.MessageTypeSequenced)
}

// SequencedMessage records the sequence metadata and executes the message
// inside.
type SequencedMessage struct{}

func (SequencedMessage) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	seq, ok := ctx.message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSequenced, ctx.message.Type())
	}

	// TODO Consider the consequences of letting users submit a sequenced message, including recursion depth and signing.

	// Basic validation
	if seq.Message == nil {
		return protocol.NewErrorStatus(seq.ID(), errors.BadRequest.With("missing message")), nil
	}

	var missing []string
	if seq.Source == nil {
		missing = append(missing, "source")
	}
	if seq.Destination == nil {
		missing = append(missing, "destination")
	}
	if seq.Number == 0 {
		missing = append(missing, "sequence number")
	}
	if len(missing) > 0 {
		return protocol.NewErrorStatus(seq.ID(), errors.BadRequest.WithFormat("invalid synthetic transaction: missing %s", strings.Join(missing, ", "))), nil
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Record the message and it's cause/produced relation
	err := batch.Message(seq.Hash()).Main().Put(seq)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	err = batch.Message(seq.Hash()).Produced().Add(seq.Message.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message produced: %w", err)
	}

	err = batch.Message(seq.Message.Hash()).Cause().Add(seq.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message cause: %w", err)
	}

	// Execute the message within
	st, err := ctx.callMessageExecutor(batch, ctx.childWith(seq.Message))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return st, nil
}
