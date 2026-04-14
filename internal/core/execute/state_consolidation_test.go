// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConsolidatedResultStructure(t *testing.T) {
	result := &ConsolidatedResult{
		BlockStateRoot: [32]byte{1, 2, 3},
		ShardResults: map[int]interface{}{
			0: "result0",
			1: "result1",
			2: "result2",
		},
		TxResults: []*ExecutionResult{
			{
				PrimaryShardID: 0,
				ShardResults:   map[int]interface{}{0: "tx0"},
			},
			{
				PrimaryShardID: 1,
				ShardResults:   map[int]interface{}{1: "tx1"},
			},
		},
		ShardsExecuted: []int{0, 1, 2},
		TotalExecutionTime: 100 * time.Millisecond,
		ShardExecutionTimes: map[int]time.Duration{
			0: 50 * time.Millisecond,
			1: 75 * time.Millisecond,
			2: 30 * time.Millisecond,
		},
	}

	// Verify structure
	require.Equal(t, [32]byte{1, 2, 3}, result.BlockStateRoot)
	require.Equal(t, 3, len(result.ShardResults))
	require.Equal(t, 2, len(result.TxResults))
	require.Equal(t, 3, len(result.ShardsExecuted))
	require.Equal(t, 100*time.Millisecond, result.TotalExecutionTime)
}

func TestComputeStatsParallelism(t *testing.T) {
	result := &ConsolidatedResult{
		BlockStateRoot: [32]byte{},
		ShardResults:   map[int]interface{}{},
		TxResults:      []*ExecutionResult{},
		ShardsExecuted: []int{0, 1, 2, 3},
		TotalExecutionTime: 100 * time.Millisecond,
		ShardExecutionTimes: map[int]time.Duration{
			0: 80 * time.Millisecond,
			1: 100 * time.Millisecond, // slowest
			2: 50 * time.Millisecond,  // fastest
			3: 70 * time.Millisecond,
		},
	}

	stats := result.ComputeStats()

	// Verify stats
	require.Equal(t, 100*time.Millisecond, stats.TotalTime)
	require.Equal(t, 2, stats.FastestShard)        // 50ms
	require.Equal(t, 1, stats.SlowestShard)        // 100ms
	require.Equal(t, 75*time.Millisecond, stats.AverageShardTime) // (80+100+50+70)/4
	require.GreaterOrEqual(t, stats.ParallelismFactor, 0.9)       // close to 1.0 (slowest/total)
	require.LessOrEqual(t, stats.ParallelismFactor, 1.1)
}

func TestComputeStatsParallelismFactor(t *testing.T) {
	t.Run("perfect parallelism", func(t *testing.T) {
		result := &ConsolidatedResult{
			BlockStateRoot:     [32]byte{},
			ShardResults:       map[int]interface{}{},
			TxResults:          []*ExecutionResult{},
			TotalExecutionTime: 100 * time.Millisecond,
			ShardExecutionTimes: map[int]time.Duration{
				0: 100 * time.Millisecond, // slowest = total
				1: 50 * time.Millisecond,
				2: 50 * time.Millisecond,
			},
		}

		stats := result.ComputeStats()
		// Perfect parallelism: slowest shard time equals total time
		require.GreaterOrEqual(t, stats.ParallelismFactor, 0.95)
		require.LessOrEqual(t, stats.ParallelismFactor, 1.05)
	})

	t.Run("poor parallelism", func(t *testing.T) {
		result := &ConsolidatedResult{
			BlockStateRoot:     [32]byte{},
			ShardResults:       map[int]interface{}{},
			TxResults:          []*ExecutionResult{},
			TotalExecutionTime: 200 * time.Millisecond, // long total
			ShardExecutionTimes: map[int]time.Duration{
				0: 100 * time.Millisecond,
				1: 100 * time.Millisecond,
			},
		}

		stats := result.ComputeStats()
		// Poor parallelism: slowest shard is much less than total
		require.Less(t, stats.ParallelismFactor, 0.6)
	})
}

func TestComputeStatsEmptyShards(t *testing.T) {
	result := &ConsolidatedResult{
		BlockStateRoot:      [32]byte{},
		ShardResults:        map[int]interface{}{},
		TxResults:           []*ExecutionResult{},
		ShardsExecuted:      []int{},
		TotalExecutionTime:  0,
		ShardExecutionTimes: map[int]time.Duration{},
	}

	stats := result.ComputeStats()

	require.Equal(t, time.Duration(0), stats.TotalTime)
	// With no shards, fastest/slowest remain -1 or 0 (uninitialized)
	// This is expected behavior for empty shard times
	require.Equal(t, time.Duration(0), stats.AverageShardTime)
}

func TestComputeStatsSingleShard(t *testing.T) {
	result := &ConsolidatedResult{
		BlockStateRoot: [32]byte{},
		ShardResults:   map[int]interface{}{},
		TxResults:      []*ExecutionResult{},
		ShardsExecuted: []int{0},
		TotalExecutionTime: 50 * time.Millisecond,
		ShardExecutionTimes: map[int]time.Duration{
			0: 50 * time.Millisecond,
		},
	}

	stats := result.ComputeStats()

	require.Equal(t, 50*time.Millisecond, stats.TotalTime)
	// With single shard, it is both fastest and slowest
	require.True(t, stats.FastestShard == 0 || stats.FastestShard == -1)
	require.True(t, stats.SlowestShard == 0 || stats.SlowestShard == -1)
	require.Equal(t, 50*time.Millisecond, stats.AverageShardTime)
	require.GreaterOrEqual(t, stats.ParallelismFactor, 0.95)
}

func TestStateConsolidatorInvalidShardID(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	consolidator := NewStateConsolidator(se, nil)

	// Try to consolidate with invalid shard ID
	badResults := map[int]*ExecutionResult{
		99: {ShardResults: map[int]interface{}{}},
	}

	_, err = consolidator.ConsolidateResults(badResults, time.Now(), map[int]time.Duration{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid shard ID")
}

func TestStateConsolidatorCreation(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	consolidator := NewStateConsolidator(se, nil)

	// Verify consolidator is created
	require.NotNil(t, consolidator)
	require.Equal(t, se, consolidator.se)
}

func TestConsolidatedResultAggregation(t *testing.T) {
	result := &ConsolidatedResult{
		BlockStateRoot: [32]byte{0xAA},
		ShardResults: map[int]interface{}{
			0: map[string]int{"count": 10},
			1: map[string]int{"count": 20},
			2: map[string]int{"count": 30},
		},
		TxResults: []*ExecutionResult{
			{PrimaryShardID: 0},
			{PrimaryShardID: 1},
			{PrimaryShardID: 2},
		},
		ShardsExecuted: []int{0, 1, 2},
		TotalExecutionTime: 150 * time.Millisecond,
		ShardExecutionTimes: map[int]time.Duration{
			0: 50 * time.Millisecond,
			1: 75 * time.Millisecond,
			2: 100 * time.Millisecond,
		},
	}

	// Verify all results are aggregated
	require.Equal(t, 3, len(result.ShardResults))
	require.Equal(t, 3, len(result.TxResults))
	require.Equal(t, 3, len(result.ShardsExecuted))

	// Verify state root is set
	require.NotEqual(t, [32]byte{}, result.BlockStateRoot)

	// Verify timing is captured
	stats := result.ComputeStats()
	require.Equal(t, 150*time.Millisecond, stats.TotalTime)
	require.Equal(t, 2, stats.SlowestShard) // 100ms
}

func TestConsolidatedResultBlockStateRoot(t *testing.T) {
	expectedRoot := [32]byte{0xDE, 0xAD, 0xBE, 0xEF}

	result := &ConsolidatedResult{
		BlockStateRoot: expectedRoot,
		ShardResults:   map[int]interface{}{},
		TxResults:      []*ExecutionResult{},
	}

	// Verify root hash is preserved
	require.Equal(t, expectedRoot, result.BlockStateRoot)
}

func TestConsolidatedResultTimingMetrics(t *testing.T) {
	startTime := time.Now()
	endTime := startTime.Add(250 * time.Millisecond)

	result := &ConsolidatedResult{
		BlockStateRoot:     [32]byte{},
		ShardResults:       map[int]interface{}{},
		TxResults:          []*ExecutionResult{},
		TotalExecutionTime: endTime.Sub(startTime),
		ShardExecutionTimes: map[int]time.Duration{
			0: 100 * time.Millisecond,
			1: 150 * time.Millisecond,
			2: 200 * time.Millisecond,
			3: 250 * time.Millisecond,
		},
	}

	stats := result.ComputeStats()

	// Verify timing metrics
	require.Equal(t, 250*time.Millisecond, stats.TotalTime)
	require.Equal(t, 0, stats.FastestShard)
	require.Equal(t, 3, stats.SlowestShard)
	require.Equal(t, 175*time.Millisecond, stats.AverageShardTime) // (100+150+200+250)/4

	// Parallelism factor: slowest (250ms) / total (250ms) ≈ 1.0 (good parallelism)
	require.Greater(t, stats.ParallelismFactor, 0.9)
	require.Less(t, stats.ParallelismFactor, 1.1)
}
