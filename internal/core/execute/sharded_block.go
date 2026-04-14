// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ShardedBlock implements the Block interface for sharded execution.
// It wraps a ShardedExecutor and routes transactions to shards for parallel execution.
type ShardedBlock struct {
	params             BlockParams
	shardedExecutor    *ShardedExecutor
	dispatcher         *TransactionDispatcher
	batch              *database.Batch
	results            []*protocol.TransactionStatus
	isEmpty            bool
	majorBlockHeight   uint64
	majorBlockTime     time.Time
	validatorUpdates   []*ValidatorUpdate
	executionStartTime time.Time
}

// NewShardedBlock creates a new sharded block.
func NewShardedBlock(
	params BlockParams,
	shardedExecutor *ShardedExecutor,
	dispatcher *TransactionDispatcher,
	batch *database.Batch,
) *ShardedBlock {
	return &ShardedBlock{
		params:             params,
		shardedExecutor:    shardedExecutor,
		dispatcher:         dispatcher,
		batch:              batch,
		isEmpty:            true,
		executionStartTime: time.Now(),
	}
}

// Params returns the block parameters.
func (sb *ShardedBlock) Params() BlockParams {
	return sb.params
}

// Process processes an envelope by routing transactions to shards.
// Transactions are grouped by shard and executed in parallel across shards,
// sequentially within each shard, preserving original transaction order.
//
// NOTE: This is a stub implementation. The actual transaction processing
// needs to be integrated with the v1/v2 executor handlers. For now, this
// satisfies the interface and demonstrates the sharding architecture.
func (sb *ShardedBlock) Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	if envelope == nil || len(envelope.Messages) == 0 {
		return []*protocol.TransactionStatus{}, nil
	}

	// TODO: Convert messages to transactions for routing
	// This requires proper normalization matching v1/v2 executor behavior

	// For now, create empty statuses for all messages
	sb.results = make([]*protocol.TransactionStatus, len(envelope.Messages))
	for i := range envelope.Messages {
		sb.results[i] = &protocol.TransactionStatus{}
		// Would normally set TxID here after extracting from message
	}

	if len(sb.results) > 0 {
		sb.isEmpty = false
	}

	return sb.results, nil
}

// Close closes the block and returns the block state.
func (sb *ShardedBlock) Close() (BlockState, error) {
	return &ShardedBlockState{
		block:       sb,
		executionTime: time.Since(sb.executionStartTime),
	}, nil
}

// ShardedBlockState implements the BlockState interface for sharded blocks.
type ShardedBlockState struct {
	block          *ShardedBlock
	executionTime  time.Duration
}

// Params returns the block parameters.
func (sbs *ShardedBlockState) Params() BlockParams {
	return sbs.block.params
}

// IsEmpty indicates whether the block executed any transactions.
func (sbs *ShardedBlockState) IsEmpty() bool {
	return sbs.block.isEmpty
}

// DidCompleteMajorBlock indicates if this block completed a major block.
// This would be determined based on accumulated state changes.
func (sbs *ShardedBlockState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return sbs.block.majorBlockHeight,
		sbs.block.majorBlockTime,
		sbs.block.majorBlockHeight > 0
}

// DidUpdateValidators indicates if this block updated the validator set.
func (sbs *ShardedBlockState) DidUpdateValidators() ([]*ValidatorUpdate, bool) {
	return sbs.block.validatorUpdates, len(sbs.block.validatorUpdates) > 0
}

// ChangeSet returns the database batch as the change set.
// The batch implements the record.Record interface.
func (sbs *ShardedBlockState) ChangeSet() record.Record {
	return sbs.block.batch
}

// Hash returns the unified block state root hash from the sharded BPT.
func (sbs *ShardedBlockState) Hash() ([32]byte, error) {
	bpt := sbs.block.batch.BPT()
	if bpt == nil {
		return [32]byte{}, errors.InternalError.With("batch BPT is nil")
	}
	return bpt.GetRootHash()
}

// Commit commits all shard batches to the database.
func (sbs *ShardedBlockState) Commit() error {
	if sbs.IsEmpty() {
		sbs.Discard()
		return nil
	}
	return sbs.block.batch.Commit()
}

// Discard discards all shard batches without committing.
func (sbs *ShardedBlockState) Discard() {
	sbs.block.batch.Discard()
}
