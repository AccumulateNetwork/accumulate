// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"context"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ExecutionResult represents the result of executing a transaction across shards.
type ExecutionResult struct {
	// ShardResults maps shard ID to the transaction result from that shard.
	// For single-shard transactions, only one entry is present.
	// For multi-shard transactions, all affected shards have entries.
	ShardResults map[int]interface{}

	// PrimaryShardID is the shard ID of the primary account being modified.
	// This is the "owner" shard for the transaction.
	PrimaryShardID int

	// SecondaryShards lists any additional shard IDs touched by this transaction.
	SecondaryShards []int

	// AffectedShards is the union of primary and secondary shards.
	AffectedShards []int
}

// ShardExecutionTask represents work to be executed on a single shard.
type ShardExecutionTask struct {
	ShardID    int
	ExecuteFn  func(shard *PerShardExecutor) (interface{}, error)
	ResultChan chan *shardExecutionResult
}

// shardExecutionResult is an internal type for collecting results from goroutines.
type shardExecutionResult struct {
	ShardID int
	Result  interface{}
	Error   error
	Panic   interface{}
}

// ExecuteTransactionOnShards executes a transaction on multiple shards in parallel.
// It spawns one goroutine per affected shard, waits for all to complete, and
// handles errors with all-or-nothing rollback semantics.
//
// If any shard fails or panics, all shards are rolled back (Discard called).
// If all shards succeed, their batches remain open for later Commit.
//
// ctx is used for cancellation. If ctx is cancelled, remaining goroutines
// will be waited for, but the operation returns an error.
func (se *ShardedExecutor) ExecuteTransactionOnShards(
	ctx context.Context,
	affectedShards []int,
	executeFn func(shard *PerShardExecutor) (interface{}, error),
) (*ExecutionResult, error) {

	if len(affectedShards) == 0 {
		return nil, errors.BadRequest.With("no shards specified for execution")
	}

	// Validate shard IDs
	for _, id := range affectedShards {
		if id < 0 || id >= se.shardCount {
			return nil, errors.BadRequest.WithFormat("invalid shard ID %d (valid range 0-%d)", id, se.shardCount-1)
		}
	}

	// Create result collection
	result := &ExecutionResult{
		ShardResults:    make(map[int]interface{}),
		PrimaryShardID:  affectedShards[0],
		SecondaryShards: affectedShards[1:],
		AffectedShards:  affectedShards,
	}

	// If only one shard, execute synchronously without goroutines
	if len(affectedShards) == 1 {
		return se.executeOnSingleShard(ctx, result, affectedShards[0], executeFn)
	}

	// For multiple shards, execute in parallel
	return se.executeOnMultipleShards(ctx, result, affectedShards, executeFn)
}

// executeOnSingleShard executes a transaction on a single shard.
// This is an optimization to avoid goroutine overhead for 80% of transactions.
func (se *ShardedExecutor) executeOnSingleShard(
	ctx context.Context,
	result *ExecutionResult,
	shardID int,
	executeFn func(shard *PerShardExecutor) (interface{}, error),
) (*ExecutionResult, error) {

	shard := se.shards[shardID]

	// Execute the function
	res, err := executeFn(shard)
	if err != nil {
		shard.Discard()
		return nil, fmt.Errorf("shard %d execution failed: %w", shardID, err)
	}

	result.ShardResults[shardID] = res
	return result, nil
}

// executeOnMultipleShards executes a transaction on multiple shards in parallel.
// Uses WaitGroup and error channels for synchronization.
func (se *ShardedExecutor) executeOnMultipleShards(
	ctx context.Context,
	result *ExecutionResult,
	affectedShards []int,
	executeFn func(shard *PerShardExecutor) (interface{}, error),
) (*ExecutionResult, error) {

	// Create channels for results and errors
	resultChan := make(chan *shardExecutionResult, len(affectedShards))
	var wg sync.WaitGroup

	// Spawn goroutine for each affected shard
	for _, shardID := range affectedShards {
		wg.Add(1)
		go se.executeShardWorker(ctx, &wg, shardID, executeFn, resultChan)
	}

	// Wait for all goroutines to complete (doesn't block on channel)
	wg.Wait()
	close(resultChan)

	// Collect results from all shards
	hasError := false
	var aggregatedErr error

	for shardResult := range resultChan {
		if shardResult.Panic != nil {
			hasError = true
			if aggregatedErr == nil {
				aggregatedErr = fmt.Errorf("shard %d panicked: %v", shardResult.ShardID, shardResult.Panic)
			}
		} else if shardResult.Error != nil {
			hasError = true
			if aggregatedErr == nil {
				aggregatedErr = fmt.Errorf("shard %d failed: %w", shardResult.ShardID, shardResult.Error)
			}
		} else {
			result.ShardResults[shardResult.ShardID] = shardResult.Result
		}
	}

	// If any shard failed, rollback all shards
	if hasError {
		se.rollbackShards(affectedShards)
		return nil, aggregatedErr
	}

	return result, nil
}

// executeShardWorker executes a transaction on a single shard within a goroutine.
// Captures panics and converts them to errors for proper error handling.
func (se *ShardedExecutor) executeShardWorker(
	ctx context.Context,
	wg *sync.WaitGroup,
	shardID int,
	executeFn func(shard *PerShardExecutor) (interface{}, error),
	resultChan chan *shardExecutionResult,
) {
	defer wg.Done()

	shard := se.shards[shardID]
	shardResult := &shardExecutionResult{ShardID: shardID}

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			shardResult.Panic = r
		}
		resultChan <- shardResult
	}()

	// Check if context was cancelled
	select {
	case <-ctx.Done():
		shardResult.Error = ctx.Err()
		return
	default:
	}

	// Execute the transaction on this shard
	res, err := executeFn(shard)
	shardResult.Result = res
	shardResult.Error = err
}

// rollbackShards discards batches on all specified shards.
// Called when any shard fails or panics during transaction execution.
func (se *ShardedExecutor) rollbackShards(shardIDs []int) {
	for _, id := range shardIDs {
		se.shards[id].Discard()
	}
}

// CommitAllShards commits all open shard batches.
// Used after successful execution across all shards.
// If any shard fails to commit, returns error and the database may be in
// a partially committed state (depends on underlying database implementation).
func (se *ShardedExecutor) CommitAllShards() error {
	for i, s := range se.shards {
		if err := s.Commit(); err != nil {
			return fmt.Errorf("commit shard %d: %w", i, err)
		}
	}
	return nil
}

// DiscardAllShards discards all open shard batches without committing.
// Used for transaction rollback or error handling.
func (se *ShardedExecutor) DiscardAllShards() {
	for _, s := range se.shards {
		s.Discard()
	}
}

// ExecutionOptions describes which shards are affected by a transaction.
// This is provided by the transaction dispatcher (Phase 2c).
type ExecutionOptions struct {
	// PrimaryShardID is the main shard (typically the source/owner account).
	PrimaryShardID int

	// SecondaryShardIDs are additional shards touched by the transaction
	// (typically recipient/target accounts).
	SecondaryShardIDs []int

	// Parallel indicates whether execution across shards should be parallelized.
	// For 40% of transactions touching 2+ shards, setting this to true
	// enables concurrent shard execution.
	Parallel bool
}

// AffectedShards returns all affected shard IDs from execution options.
func (eo *ExecutionOptions) AffectedShards() []int {
	shards := []int{eo.PrimaryShardID}
	return append(shards, eo.SecondaryShardIDs...)
}
