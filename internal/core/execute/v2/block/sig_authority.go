// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[AuthoritySignature](&signatureExecutors, protocol.SignatureTypeAuthority)
}

// AuthoritySignature processes delegated signatures.
type AuthoritySignature struct{}

func (x AuthoritySignature) Process(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	_, ok := ctx.signature.(*protocol.AuthoritySignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid signature type: expected %v, got %v", protocol.SignatureTypeAuthority, ctx.signature.Type())
	}

	// An authority signature MUST NOT be submitted directly
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("a non-synthetic message cannot carry an %v signature", ctx.signature.Type())), nil
	}

	// Make sure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// TODO Implement authority signatures
	status := new(protocol.TransactionStatus)
	status.TxID = ctx.message.ID()
	status.Code = errors.Delivered
	return status, nil
}
