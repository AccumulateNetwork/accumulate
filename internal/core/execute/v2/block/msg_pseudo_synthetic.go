// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[PseudoSynthetic](&messageExecutors, internal.MessageTypePseudoSynthetic)
}

// PseudoSynthetic executes a message.
type PseudoSynthetic struct{}

func (PseudoSynthetic) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, errors.InternalError.With("invalid attempt to validate an internal message")
}

func (PseudoSynthetic) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, ok := ctx.message.(*internal.PseudoSynthetic)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypePseudoSynthetic, ctx.message.Type())
	}

	if msg.Message == nil {
		return nil, errors.InternalError.With("missing message")
	}

	// Process the message
	st, err := ctx.callMessageExecutor(batch, msg.Message)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return st, nil
}
