// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package abci implements the Accumulate ABCI applications.
//
// # Transaction Processing
//
// Tendermint processes transactions in the following phases:
//
//   - BeginBlock
//   - [CheckTx]
//   - [DeliverTx]
//   - EndBlock
//   - Commit
package abci

import (
	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Version is the version of the ABCI applications.
const Version uint64 = 0x1

type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)

	LoadStateRoot(*database.Batch) ([]byte, error)
	RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error
	InitChainValidators(initVal []abci.ValidatorUpdate) (additional [][]byte, err error)

	ValidateEnvelopeSet(batch *database.Batch, deliveries []*chain.Delivery, captureError func(error, *chain.Delivery, *protocol.TransactionStatus)) []*protocol.TransactionStatus
	BeginBlock(block *block.Block) error
	ExecuteEnvelopeSet(block *block.Block, deliveries []*chain.Delivery, captureError func(error, *chain.Delivery, *protocol.TransactionStatus)) []*protocol.TransactionStatus
	EndBlock(block *block.Block) error
}
