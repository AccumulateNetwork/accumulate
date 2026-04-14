// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ShardedExecutorWrapper wraps a ShardedExecutor and implements the Executor interface.
// It provides the glue between the sharded executor infrastructure and the main
// block execution pipeline.
type ShardedExecutorWrapper struct {
	shardCount int
	database   database.Beginner
	options    Options
	logger     logging.OptionalLogger

	// Wrapped components
	shardedExecutor    *ShardedExecutor
	dispatcher         *TransactionDispatcher
}

// NewShardedExecutorWrapper creates a new sharded executor wrapper.
// shardCount must be a power of 2 between 4 and 256.
func NewShardedExecutorWrapper(opts Options, shardCount int) (*ShardedExecutorWrapper, error) {
	// Validate shard count
	if shardCount < 4 || shardCount > 256 {
		return nil, errors.BadRequest.WithFormat("shard count must be between 4 and 256, got %d", shardCount)
	}
	if shardCount&(shardCount-1) != 0 {
		return nil, errors.BadRequest.WithFormat("shard count must be a power of 2, got %d", shardCount)
	}

	// Create the sharded executor
	se, err := NewShardedExecutor(shardCount, opts.Database)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create sharded executor: %w", err)
	}

	// Create the transaction dispatcher with the same shard depth
	shardDepth := bits.TrailingZeros(uint(shardCount))
	dispatcher := NewTransactionDispatcher(shardDepth)

	wrapper := &ShardedExecutorWrapper{
		shardCount:      shardCount,
		database:        opts.Database,
		options:         opts,
		shardedExecutor: se,
		dispatcher:      dispatcher,
	}

	if opts.Logger != nil {
		wrapper.logger.L = opts.Logger.With("module", "sharded-executor")
	}

	return wrapper, nil
}

// EnableTimers enables block timers (no-op for now, would need per-shard timers).
func (sew *ShardedExecutorWrapper) EnableTimers() {
	// TODO: Implement per-shard timing
}

// StoreBlockTimers stores block timer data (no-op for now).
func (sew *ShardedExecutorWrapper) StoreBlockTimers(ds *logging.DataSet) {
	// TODO: Aggregate and store per-shard timers
}

// LastBlock returns the height and hash of the last block.
// This reads from the database to get the last known block state.
func (sew *ShardedExecutorWrapper) LastBlock() (*BlockParams, [32]byte, error) {
	batch := sew.database.Begin(false)
	defer batch.Discard()

	// TODO: Implement proper last block retrieval from ledger
	// For now, return empty/zero values - this needs integration with actual ledger

	return &BlockParams{}, [32]byte{}, errors.NotFound
}

// Init initializes the executor with a set of validators.
// This validates the initial validator set and returns any additional validators.
func (sew *ShardedExecutorWrapper) Init(validators []*ValidatorUpdate) (additional []*ValidatorUpdate, err error) {
	// TODO: Implement validator initialization
	// For now, just return the validators as-is

	return validators, nil
}

// Validate validates a set of messages without executing them.
// This pre-validates transactions before they are processed in a block.
func (sew *ShardedExecutorWrapper) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
	// TODO: Implement message validation
	// This should validate the format and signatures of messages without executing them

	if envelope == nil {
		return nil, errors.BadRequest.With("envelope is nil")
	}

	statuses := make([]*protocol.TransactionStatus, len(envelope.Messages))
	for i := range envelope.Messages {
		statuses[i] = &protocol.TransactionStatus{}
	}

	return statuses, nil
}

// Begin creates a new block and begins the execution context.
// This prepares shards for transaction execution.
func (sew *ShardedExecutorWrapper) Begin(params BlockParams) (Block, error) {
	// Open a new batch for the block
	batch := sew.database.Begin(true)

	// Initialize all shards with their own batches
	sew.shardedExecutor.BeginBlock()

	// Create the sharded block wrapper
	shardedBlock := NewShardedBlock(params, sew.shardedExecutor, sew.dispatcher, batch)

	return shardedBlock, nil
}
