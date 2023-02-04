// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[KeySignature](&signatureExecutors,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy,
		protocol.SignatureTypeETH,
	)
}

// KeySignature processes key signatures.
type KeySignature struct{}

func (KeySignature) Process(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	status := new(protocol.TransactionStatus)
	status.TxID = ctx.message.ID()
	status.Received = ctx.Block.Index

	s, err := ctx.Executor.processSignature2(batch, &chain.Delivery{
		Transaction: ctx.transaction,
		Forwarded:   ctx.forwarded.Has(ctx.message.ID().Hash()),
	}, ctx.signature)
	ctx.Block.State.MergeSignature(s)
	if err == nil {
		status.Code = errors.Delivered
	} else {
		status.Set(err)
	}

	return status, nil
}
