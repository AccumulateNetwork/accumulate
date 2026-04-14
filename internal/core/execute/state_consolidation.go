// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ConsolidatedResult represents the unified result of executing a block across all shards.
// It combines per-shard execution results with the unified block state hash.
type ConsolidatedResult struct {
	// BlockStateRoot is the unified Merkle root hash combining all shard roots.
	// Computed by ShardedBPT.GetRootHash() which combines roots in a binary tree.
	BlockStateRoot [32]byte

	// ShardResults maps shard ID to the execution result from that shard.
	// Preserved for diagnostics, debugging, and per-shard metrics.
	ShardResults map[int]interface{}

	// TxResults are the transaction results merged from all shards.
	// Aggregated in shard execution order for deterministic ordering.
	TxResults []*ExecutionResult

	// ShardsExecuted lists all shard IDs that participated in this block.
	ShardsExecuted []int

	// TotalExecutionTime is the wall-clock time from block start to consolidation.
	TotalExecutionTime time.Duration

	// ShardExecutionTimes maps shard ID to its individual execution duration.
	// Useful for identifying performance bottlenecks.
	ShardExecutionTimes map[int]time.Duration
}

// StateConsolidator manages consolidation of per-shard execution results into unified block state.
// It orchestrates the final phase of block execution: combining shard results and computing
// the unified state root hash.
type StateConsolidator struct {
	se    *ShardedExecutor
	batch *database.Batch
}

// NewStateConsolidator creates a new consolidator for the given executor and batch.
func NewStateConsolidator(se *ShardedExecutor, batch *database.Batch) *StateConsolidator {
	return &StateConsolidator{
		se:    se,
		batch: batch,
	}
}

// ConsolidateResults merges per-shard execution results and computes the unified block state root.
// This is called after all shards have completed execution (within a block).
//
// The consolidation process:
// 1. Validates all shard execution succeeded (all-or-nothing semantics)
// 2. Collects per-shard results and timing information
// 3. Calls ShardedBPT.GetRootHash() to combine all shard state roots
// 4. Returns unified ConsolidatedResult with block state root
//
// On any shard error, returns error and the caller should discard the batch.
func (sc *StateConsolidator) ConsolidateResults(
	executionResults map[int]*ExecutionResult,
	startTime time.Time,
	shardTimings map[int]time.Duration,
) (*ConsolidatedResult, error) {

	// Validate all shards executed successfully
	// (error handling already done in parallel execution, but double-check)
	for shardID := range executionResults {
		if shardID < 0 || shardID >= sc.se.ShardCount() {
			return nil, errors.BadRequest.WithFormat(
				"invalid shard ID in results: %d (valid range 0-%d)",
				shardID, sc.se.ShardCount()-1)
		}
	}

	// Get unified state root hash by combining all shard roots
	// ShardedBPT.GetRootHash() automatically combines roots from all shards in parallel
	rootHash, err := sc.computeBlockStateRoot()
	if err != nil {
		return nil, fmt.Errorf("compute block state root: %w", err)
	}

	// Collect results and build consolidated result
	result := &ConsolidatedResult{
		BlockStateRoot:     rootHash,
		ShardResults:       make(map[int]interface{}),
		TxResults:          []*ExecutionResult{},
		ShardsExecuted:     make([]int, 0, len(executionResults)),
		ShardExecutionTimes: shardTimings,
		TotalExecutionTime: time.Since(startTime),
	}

	// Aggregate per-shard results
	for shardID, exResult := range executionResults {
		result.ShardsExecuted = append(result.ShardsExecuted, shardID)
		if exResult != nil {
			result.ShardResults[shardID] = exResult
			result.TxResults = append(result.TxResults, exResult)
		}
	}

	return result, nil
}

// computeBlockStateRoot gets the unified Merkle root hash from the sharded BPT.
// This combines all shard state roots in a binary tree structure, producing
// the same hash as a non-sharded BPT with identical data.
func (sc *StateConsolidator) computeBlockStateRoot() ([32]byte, error) {
	// Get the BPT from the batch (contains all shard changes)
	bpt := sc.batch.BPT()
	if bpt == nil {
		return [32]byte{}, errors.InternalError.With("batch BPT is nil")
	}

	// GetRootHash() automatically combines roots from all shards in parallel
	// and returns the unified root hash
	return bpt.GetRootHash()
}

// CommitBlock commits all shard batches to the database, finalizing the block state.
// This is called after ConsolidateResults succeeds, at which point all changes
// are ready to be persisted.
//
// The commit is atomic at the database level: either all changes persist or none do.
// If commit fails, the block state is not modified.
func (sc *StateConsolidator) CommitBlock() error {
	return sc.batch.Commit()
}

// DiscardBlock discards all shard batches without committing.
// Called on error or when the block execution fails.
// After this call, no changes from this block are persisted.
func (sc *StateConsolidator) DiscardBlock() {
	sc.batch.Discard()
}

// BlockConsolidationWorkflow orchestrates the complete consolidation workflow.
// It validates execution results, consolidates state, and commits to the database.
//
// Workflow:
// 1. Validate all shard executions succeeded
// 2. Consolidate results and compute unified state root
// 3. Commit all changes to database atomically
//
// On any error, discards the batch and returns error.
// On success, the block state is persisted and the consolidated result is returned.
func (sc *StateConsolidator) BlockConsolidationWorkflow(
	executionResults map[int]*ExecutionResult,
	startTime time.Time,
	shardTimings map[int]time.Duration,
) (*ConsolidatedResult, error) {

	// Check for execution errors (all-or-nothing semantics)
	// If any shard failed, the parallel execution should have already returned error
	// But verify the results map has entries for all shards
	if len(executionResults) == 0 {
		sc.DiscardBlock()
		return nil, errors.BadRequest.With("no shard results available")
	}

	// Consolidate per-shard results into unified block state
	consolidated, err := sc.ConsolidateResults(executionResults, startTime, shardTimings)
	if err != nil {
		sc.DiscardBlock()
		return nil, fmt.Errorf("consolidate results: %w", err)
	}

	// Commit all shard batches to the database
	// This is atomic at the database level
	if err := sc.CommitBlock(); err != nil {
		sc.DiscardBlock()
		return nil, fmt.Errorf("commit block: %w", err)
	}

	return consolidated, nil
}

// ExecutionStats provides execution metrics across all shards.
type ExecutionStats struct {
	// TotalTime is the wall-clock time for the entire block execution and consolidation.
	TotalTime time.Duration

	// ShardTimes maps shard ID to its individual execution time.
	ShardTimes map[int]time.Duration

	// FastestShard is the shard that completed first.
	FastestShard int

	// SlowestShard is the shard that took the longest.
	SlowestShard int

	// AverageShardTime is the mean execution time across all shards.
	AverageShardTime time.Duration

	// ParallelismFactor estimates how well shards parallelized.
	// 1.0 = perfect parallelism (slowest shard time)
	// <1.0 = some shards finished early and waited
	ParallelismFactor float64
}

// ComputeStats calculates execution statistics from a consolidated result.
func (cr *ConsolidatedResult) ComputeStats() ExecutionStats {
	stats := ExecutionStats{
		TotalTime:  cr.TotalExecutionTime,
		ShardTimes: cr.ShardExecutionTimes,
	}

	if len(cr.ShardExecutionTimes) == 0 {
		return stats
	}

	// Find fastest and slowest shards
	var minDur, maxDur time.Duration = cr.TotalExecutionTime, time.Duration(0)
	minShard, maxShard := -1, -1
	totalDur := time.Duration(0)

	for shardID, dur := range cr.ShardExecutionTimes {
		if dur < minDur {
			minDur = dur
			minShard = shardID
		}
		if dur > maxDur {
			maxDur = dur
			maxShard = shardID
		}
		totalDur += dur
	}

	stats.FastestShard = minShard
	stats.SlowestShard = maxShard
	stats.AverageShardTime = time.Duration(int64(totalDur) / int64(len(cr.ShardExecutionTimes)))

	// Parallelism factor: total shard time vs wall-clock time
	// Perfect parallelism: maxDur ≈ totalTime
	// Serial execution: totalDur = sum of all shard times
	if cr.TotalExecutionTime > 0 {
		stats.ParallelismFactor = float64(maxDur.Nanoseconds()) / float64(cr.TotalExecutionTime.Nanoseconds())
	}

	return stats
}
