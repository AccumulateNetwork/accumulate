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
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An Executor creates blocks and executes messages.
type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)

	// LoadStateRoot returns the BPT root hash.
	LoadStateRoot(*database.Batch) ([]byte, error)

	// RestoreSnapshot restores the database from a snapshot.
	RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error

	// InitChainValidators validates the given initial validators and returns
	// any additional validators.
	InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error)

	// Validate validates a set of messages.
	Validate(*database.Batch, []messaging.Message) ([]*protocol.TransactionStatus, error)

	// Begin begins a Tendermint block.
	Begin(BlockParams) (Block, error)
}

// Options are the options for constructing an [Executor]
type Options struct {
	Logger                 log.Logger                         //
	Database               database.Beginner                  //
	Key                    ed25519.PrivateKey                 // Private validator key
	Router                 routing.Router                     //
	Describe               config.Describe                    // Network description
	EventBus               *events.Bus                        //
	MajorBlockScheduler    blockscheduler.MajorBlockScheduler //
	BackgroundTaskLauncher func(func())                       // Background task launcher
	NewDispatcher          func() Dispatcher                  // Synthetic transaction dispatcher factory
	Sequencer              private.Sequencer                  // Synthetic and anchor sequence API service
	Querier                api.Querier                        // Query API service
}

// A Dispatcher dispatches synthetic transactions produced by the executor.
type Dispatcher interface {
	// Submit adds an envelope to the queue.
	Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error

	// Send submits the queued transactions.
	Send(context.Context) <-chan error
}

// BlockParams are the parameters for a new [Block].
type BlockParams struct {
	Context    context.Context
	IsLeader   bool
	Index      uint64
	Time       time.Time
	CommitInfo *abcitypes.CommitInfo
	Evidence   []abcitypes.Misbehavior
}

// A Block is the context in which messages are processed.
type Block interface {
	// Params returns the parameters the block was created with.
	Params() BlockParams

	// Process processes a set of messages.
	Process([]messaging.Message) ([]*protocol.TransactionStatus, error)

	// Close closes the block and returns the end state of the block.
	Close() (BlockState, error)
}

// BlockState is the state of a completed block.
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
