// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// An Executor creates blocks and executes messages.
type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)

	// LastBlock returns the height and hash of the last block.
	LastBlock() (*BlockParams, [32]byte, error)

	// Init must be called after the database is initialized from the genesis
	// snapshot. Init validates the initial validators and returns any
	// additional validators.
	Init(validators []*ValidatorUpdate) (additional []*ValidatorUpdate, err error)

	// Validate validates a set of messages.
	Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error)

	// Begin begins a Tendermint block.
	Begin(BlockParams) (Block, error)
}

type ValidatorUpdate struct {
	Type      protocol.SignatureType
	PublicKey []byte
	Power     int64
}

func (v *ValidatorUpdate) Equal(u *ValidatorUpdate) bool {
	return v.Type == u.Type &&
		bytes.Equal(v.PublicKey, u.PublicKey) &&
		v.Power == u.Power
}

// Options are the options for constructing an [Executor]
type Options struct {
	Logger                 log.Logger         //
	Database               database.Beginner  //
	Key                    ed25519.PrivateKey // Private validator key
	Router                 routing.Router     //
	Describe               DescribeShim       // Network description
	EventBus               *events.Bus        //
	BackgroundTaskLauncher func(func())       // Background task launcher
	NewDispatcher          func() Dispatcher  // Synthetic transaction dispatcher factory
	Sequencer              private.Sequencer  // Synthetic and anchor sequence API service
	Querier                api.Querier        // Query API service
	EnableHealing          bool               //
}

// A Dispatcher dispatches synthetic transactions produced by the executor.
type Dispatcher interface {
	// Submit adds an envelope to the queue.
	Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error

	// Send submits the queued transactions.
	Send(context.Context) <-chan error

	// Close closes the dispatcher.
	Close()
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
	Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error)

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

	// DidUpdateValidators indicates that this block updated the validator set.
	DidUpdateValidators() ([]*ValidatorUpdate, bool)

	// ChangeSet is the database batch.
	ChangeSet() record.Record

	// Hash returns the block hash.
	Hash() ([32]byte, error)

	// Commit commits changes made by this block.
	Commit() error

	// Discard discards changes made by this block.
	Discard()
}

func NewDatabaseObserver() database.Observer {
	return internal.NewDatabaseObserver()
}
