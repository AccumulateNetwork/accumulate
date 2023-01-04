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
	"context"
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Version is the version of the ABCI applications.
const Version uint64 = 0x1

type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)
	SetBackgroundTaskManager(f func(func()))

	LoadStateRoot(*database.Batch) ([]byte, error)
	RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error
	InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error)

	// Validate validates a message.
	Validate(*database.Batch, messaging.Message) (*protocol.TransactionStatus, error)

	// Begin begins a block.
	Begin(*database.Batch, BlockParams) (Block, error)
}

type BlockParams struct {
	Context    context.Context
	IsLeader   bool
	Index      uint64
	Time       time.Time
	CommitInfo *abcitypes.CommitInfo
	Evidence   []abcitypes.Misbehavior
}

type Block interface {
	// Params returns the parameters the block was created with.
	Params() BlockParams

	// Process processes a message.
	Process(messaging.Message) (*protocol.TransactionStatus, error)

	// Close closes the block and returns the end state of the block.
	Close() (BlockState, error)
}

type BlockState interface {
	// Params returns the parameters the block was created with.
	Params() BlockParams
	
	// IsEmpty indicates that nothing happened in this block.
	IsEmpty() bool

	// DidCompleteMajorBlock indicates that this block completed a major block.
	DidCompleteMajorBlock() (uint64, time.Time, bool)

	// Commit commits changes made by this block.
	Commit() error

	// Discard discards changes made by this block.
	Discard()
}

func ValidateEnvelopeSet(x Executor, batch *database.Batch, deliveries []messaging.Message) []*protocol.TransactionStatus {
	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		status, err := x.Validate(batch, delivery)
		if status == nil {
			status = new(protocol.TransactionStatus)
		}
		results[i] = status

		if err != nil {
			status.Set(err)
		}
	}

	return results
}

func ExecuteEnvelopeSet(block Block, deliveries []messaging.Message) []*protocol.TransactionStatus {
	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		status, err := block.Process(delivery)
		if status == nil {
			status = new(protocol.TransactionStatus)
		}
		results[i] = status

		if err != nil {
			status.Set(err)
		}
	}

	return results
}
