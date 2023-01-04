// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"context"
	"crypto/ed25519"
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)

	LoadStateRoot(*database.Batch) ([]byte, error)
	RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error
	InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error)

	// Validate validates a message.
	Validate(*database.Batch, messaging.Message) (*protocol.TransactionStatus, error)

	// Begin begins a block.
	Begin(*database.Batch, BlockParams) (Block, error)
}

type Options struct {
	Logger                 log.Logger                         //
	Database               database.Beginner                  //
	Key                    ed25519.PrivateKey                 // Private validator key
	Router                 routing.Router                     //
	Describe               config.Describe                    // Network description
	EventBus               *events.Bus                        //
	MajorBlockScheduler    blockscheduler.MajorBlockScheduler //
	BackgroundTaskLauncher func(func())                       // Background task launcher
	BatchReplayLimit       int                                //
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
