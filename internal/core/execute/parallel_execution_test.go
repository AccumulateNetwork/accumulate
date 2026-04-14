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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecuteTransactionOnSingleShard(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Define a simple execution function that doesn't require a batch
	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		return fmt.Sprintf("shard_%d", shard.ID), nil
	}

	// Execute on single shard
	result, err := se.ExecuteTransactionOnShards(context.Background(), []int{0}, executeFn)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, len(result.ShardResults))
	require.Equal(t, "shard_0", result.ShardResults[0])
	require.Equal(t, 0, result.PrimaryShardID)
	require.Empty(t, result.SecondaryShards)
}

func TestExecuteTransactionOnMultipleShards(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Execution function that records which shards executed
	var mu sync.Mutex
	executed := make([]int, 0)

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		mu.Lock()
		executed = append(executed, shard.ID)
		mu.Unlock()
		return shard.ID, nil
	}

	// Execute on multiple shards
	affectedShards := []int{0, 2, 3}
	result, err := se.ExecuteTransactionOnShards(context.Background(), affectedShards, executeFn)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, len(result.ShardResults))
	require.Equal(t, 0, result.PrimaryShardID)
	require.Equal(t, []int{2, 3}, result.SecondaryShards)

	// Verify all shards executed
	require.ElementsMatch(t, affectedShards, executed)
}

func TestExecuteTransactionErrorRollback(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Execution function that fails on shard 2
	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		if shard.ID == 2 {
			return nil, fmt.Errorf("shard 2 error")
		}
		return shard.ID, nil
	}

	// Execute on multiple shards (should rollback all on failure)
	affectedShards := []int{0, 2, 3}
	result, err := se.ExecuteTransactionOnShards(context.Background(), affectedShards, executeFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "shard 2 error")
	require.Nil(t, result)
}

func TestExecuteTransactionPanicRecovery(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Execution function that panics on shard 1
	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		if shard.ID == 1 {
			panic("shard 1 panic")
		}
		return shard.ID, nil
	}

	// Execute on multiple shards (should recover from panic and rollback)
	affectedShards := []int{0, 1, 2}
	result, err := se.ExecuteTransactionOnShards(context.Background(), affectedShards, executeFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "shard 1 panicked")
	require.Nil(t, result)
}

func TestExecuteTransactionContextCancellation(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Execution function that sleeps (to allow cancellation)
	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return shard.ID, nil
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	affectedShards := []int{0, 1, 2}
	result, err := se.ExecuteTransactionOnShards(ctx, affectedShards, executeFn)
	require.Error(t, err)
	require.Nil(t, result)
}

func TestExecuteTransactionInvalidShardID(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		return shard.ID, nil
	}

	// Try to execute on invalid shard ID
	result, err := se.ExecuteTransactionOnShards(context.Background(), []int{99}, executeFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid shard ID")
	require.Nil(t, result)
}

func TestExecuteTransactionEmptyShards(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		return shard.ID, nil
	}

	// Try to execute with no shards
	result, err := se.ExecuteTransactionOnShards(context.Background(), []int{}, executeFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no shards specified")
	require.Nil(t, result)
}

func TestExecutionResultStructure(t *testing.T) {
	se, err := NewShardedExecutor(8, nil)
	require.NoError(t, err)

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		return map[string]interface{}{
			"shard":     shard.ID,
			"timestamp": time.Now(),
		}, nil
	}

	affectedShards := []int{1, 3, 5}
	result, err := se.ExecuteTransactionOnShards(context.Background(), affectedShards, executeFn)
	require.NoError(t, err)

	// Verify result structure
	require.Equal(t, 1, result.PrimaryShardID)
	require.Equal(t, []int{3, 5}, result.SecondaryShards)
	require.Equal(t, []int{1, 3, 5}, result.AffectedShards)

	// Verify all shards have results
	for _, shardID := range affectedShards {
		require.Contains(t, result.ShardResults, shardID)
		shardResult := result.ShardResults[shardID].(map[string]interface{})
		require.Equal(t, shardID, shardResult["shard"])
	}
}

func TestParallelExecutionOrdering(t *testing.T) {
	// Test that multiple shards truly execute in parallel
	// by measuring execution time
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Execution function that sleeps for a fixed time
	const sleepDuration = 50 * time.Millisecond
	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		time.Sleep(sleepDuration)
		return shard.ID, nil
	}

	affectedShards := []int{0, 1, 2, 3}

	// Measure execution time
	start := time.Now()
	result, err := se.ExecuteTransactionOnShards(context.Background(), affectedShards, executeFn)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)

	// If executed serially, would take 4 * sleepDuration = 200ms
	// If executed in parallel, should take ~sleepDuration = 50ms
	// Allow some overhead but verify parallelism
	require.Less(t, elapsed, sleepDuration*2, "execution took too long; shards may not be parallel")
}

func TestExecutionOptionsAffectedShards(t *testing.T) {
	opts := ExecutionOptions{
		PrimaryShardID:    1,
		SecondaryShardIDs: []int{3, 5, 7},
		Parallel:          true,
	}

	affected := opts.AffectedShards()
	require.Equal(t, []int{1, 3, 5, 7}, affected)
}

func TestExecutionOptionsEmptySecondary(t *testing.T) {
	opts := ExecutionOptions{
		PrimaryShardID:    2,
		SecondaryShardIDs: []int{},
		Parallel:          false,
	}

	affected := opts.AffectedShards()
	require.Equal(t, []int{2}, affected)
}
