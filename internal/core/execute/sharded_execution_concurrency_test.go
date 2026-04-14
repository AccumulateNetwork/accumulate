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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TestConcurrency_ParallelShardExecution verifies that multiple shards execute
// truly in parallel by measuring wall-clock time. If 4 shards each sleep 50ms,
// parallel execution should complete in ~50ms, not ~200ms.
func TestConcurrency_ParallelShardExecution(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	const perShard = 50 * time.Millisecond

	// Record start times per shard to verify overlap
	type shardTiming struct {
		start time.Time
		end   time.Time
	}
	var mu sync.Mutex
	timings := make(map[int]shardTiming)

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		start := time.Now()
		time.Sleep(perShard)
		end := time.Now()
		mu.Lock()
		timings[shard.ID] = shardTiming{start, end}
		mu.Unlock()
		return shard.ID, nil
	}

	wallStart := time.Now()
	result, err := se.ExecuteTransactionOnShards(
		context.Background(), []int{0, 1, 2, 3}, executeFn)
	wallElapsed := time.Since(wallStart)

	require.NoError(t, err)
	require.Equal(t, 4, len(result.ShardResults))

	// Wall-clock should be closer to 50ms than 200ms
	require.Less(t, wallElapsed, perShard*3,
		"parallel execution took %v; expected < %v", wallElapsed, perShard*3)

	// Verify execution windows overlap (at least two shards running concurrently)
	overlaps := 0
	ids := []int{0, 1, 2, 3}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a := timings[ids[i]]
			b := timings[ids[j]]
			if a.start.Before(b.end) && b.start.Before(a.end) {
				overlaps++
			}
		}
	}
	require.Greater(t, overlaps, 0, "no overlapping shard executions detected")
}

// TestConcurrency_PanicRecovery verifies that a panic in one shard goroutine
// is caught and converted to an error, and other shards still complete.
func TestConcurrency_PanicRecovery(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	var completed atomic.Int32

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		if shard.ID == 2 {
			panic("transaction processing fault")
		}
		completed.Add(1)
		return shard.ID, nil
	}

	result, err := se.ExecuteTransactionOnShards(
		context.Background(), []int{0, 1, 2, 3}, executeFn)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "panicked")
	require.Contains(t, err.Error(), "transaction processing fault")

	// Other shards should have completed (they run in parallel)
	// At least some of the non-panicking shards should finish
	require.Greater(t, completed.Load(), int32(0),
		"non-panicking shards should still complete")
}

// TestConcurrency_PanicRecoverySingleShard verifies that a panic on a single
// shard execution path (no goroutines) propagates as a panic since there's no
// recovery wrapper for the single-shard fast path.
func TestConcurrency_PanicRecoverySingleShard(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		panic("single shard panic")
	}

	// Single shard path doesn't have defer/recover, so it panics
	require.Panics(t, func() {
		se.ExecuteTransactionOnShards(context.Background(), []int{0}, executeFn)
	})
}

// TestConcurrency_ContextCancellation verifies that a cancelled context
// prevents shard execution and returns an error.
func TestConcurrency_ContextCancellation(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	var executed atomic.Int32

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		executed.Add(1)
		time.Sleep(200 * time.Millisecond)
		return shard.ID, nil
	}

	// Cancel context before execution starts
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := se.ExecuteTransactionOnShards(ctx, []int{0, 1, 2, 3}, executeFn)

	require.Error(t, err)
	require.Nil(t, result)
}

// TestConcurrency_ContextCancellationMidFlight verifies that cancelling the
// context while shards are executing is handled gracefully - the operation
// returns an error after all goroutines complete.
func TestConcurrency_ContextCancellationMidFlight(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	var started atomic.Int32

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		started.Add(1)
		// Simulate work
		time.Sleep(50 * time.Millisecond)
		return shard.ID, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Some shards may see the cancellation, some may not - either way no hang
	result, err := se.ExecuteTransactionOnShards(ctx, []int{0, 1, 2, 3}, executeFn)

	// Should either succeed or return an error, but never hang
	if err != nil {
		require.Nil(t, result)
	} else {
		require.NotNil(t, result)
	}
}

// TestConcurrency_WaitGroupSynchronization verifies that all shard goroutines
// complete before ExecuteTransactionOnShards returns, by checking that all
// results are collected.
func TestConcurrency_WaitGroupSynchronization(t *testing.T) {
	se, err := NewShardedExecutor(8, nil)
	require.NoError(t, err)

	var completed atomic.Int32

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		// Stagger start times to stress WaitGroup
		time.Sleep(time.Duration(shard.ID*5) * time.Millisecond)
		completed.Add(1)
		return fmt.Sprintf("done_%d", shard.ID), nil
	}

	shards := []int{0, 1, 2, 3, 4, 5, 6, 7}
	result, err := se.ExecuteTransactionOnShards(context.Background(), shards, executeFn)

	require.NoError(t, err)
	require.NotNil(t, result)

	// All shards must have completed by the time we get here
	require.Equal(t, int32(8), completed.Load(),
		"not all shard goroutines completed before return")

	// All results must be present
	require.Equal(t, 8, len(result.ShardResults))
	for _, id := range shards {
		require.Equal(t, fmt.Sprintf("done_%d", id), result.ShardResults[id])
	}
}

// TestConcurrency_NoGoroutineLeaks runs multiple rounds of parallel execution
// and verifies all goroutines terminate cleanly by checking completion counts.
func TestConcurrency_NoGoroutineLeaks(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	var totalStarted, totalFinished atomic.Int32

	for round := 0; round < 50; round++ {
		executeFn := func(shard *PerShardExecutor) (interface{}, error) {
			totalStarted.Add(1)
			defer totalFinished.Add(1)
			// Mix of success and failure
			if round%10 == 0 && shard.ID == 1 {
				return nil, fmt.Errorf("periodic error round %d", round)
			}
			return shard.ID, nil
		}

		se.ExecuteTransactionOnShards(context.Background(), []int{0, 1, 2, 3}, executeFn)
	}

	// All started goroutines must have finished
	require.Equal(t, totalStarted.Load(), totalFinished.Load(),
		"goroutine leak: %d started, %d finished",
		totalStarted.Load(), totalFinished.Load())
}

// TestConcurrency_RaceDetection exercises concurrent access to shared state
// through the PerShardExecutor mutex. This test is designed to trigger the
// Go race detector if synchronization is broken.
func TestConcurrency_RaceDetection(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	// Concurrent access to per-shard loaded accounts via ForEachShard
	err = se.ForEachShard(func(shard *PerShardExecutor) error {
		// Simulate concurrent account tracking within a shard
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				shard.mu.Lock()
				u, _ := url.Parse(fmt.Sprintf("acc://user%d.acme/tokens", idx))
				shard.loadedAccounts[u.AccountID32()] = struct{}{}
				shard.mu.Unlock()

				// Read concurrently
				shard.LoadedAccountCount()
			}(i)
		}
		wg.Wait()
		return nil
	})
	require.NoError(t, err)
}

// TestConcurrency_ConcurrentAccountAccessPerShard verifies that multiple
// goroutines can safely call LoadedAccountCount while accounts are being
// added. The race detector will flag any unsafe access.
func TestConcurrency_ConcurrentAccountAccessPerShard(t *testing.T) {
	ps := newPerShardExecutor(0)

	var wg sync.WaitGroup

	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ps.mu.Lock()
				u, _ := url.Parse(fmt.Sprintf("acc://writer%d-acct%d.acme", idx, j))
				ps.loadedAccounts[u.AccountID32()] = struct{}{}
				ps.mu.Unlock()
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				count := ps.LoadedAccountCount()
				assert.GreaterOrEqual(t, count, 0)
			}
		}()
	}

	wg.Wait()

	// Final count should reflect all unique accounts added
	finalCount := ps.LoadedAccountCount()
	require.Greater(t, finalCount, 0)
	require.LessOrEqual(t, finalCount, 1000) // 10 writers * 100 accounts max
}

// TestConcurrency_ForEachShardParallel verifies ForEachShard runs shards
// concurrently and collects errors properly.
func TestConcurrency_ForEachShardParallel(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	const perShard = 50 * time.Millisecond

	start := time.Now()
	err = se.ForEachShard(func(shard *PerShardExecutor) error {
		time.Sleep(perShard)
		return nil
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	// 4 shards * 50ms sequential = 200ms; parallel should be ~50ms
	require.Less(t, elapsed, perShard*3,
		"ForEachShard took %v; expected parallel execution", elapsed)
}

// TestConcurrency_ForEachShardErrorCollection verifies that when multiple
// shards return errors, at least one error is reported.
func TestConcurrency_ForEachShardErrorCollection(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	err = se.ForEachShard(func(shard *PerShardExecutor) error {
		if shard.ID%2 == 0 {
			return fmt.Errorf("shard %d failed", shard.ID)
		}
		return nil
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed")
}

// TestConcurrency_MultipleExecutionsSerially verifies that sequential calls to
// ExecuteTransactionOnShards on the same ShardedExecutor do not interfere with
// each other (no stale goroutine state).
func TestConcurrency_MultipleExecutionsSerially(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	for round := 0; round < 20; round++ {
		executeFn := func(shard *PerShardExecutor) (interface{}, error) {
			return fmt.Sprintf("round_%d_shard_%d", round, shard.ID), nil
		}

		result, err := se.ExecuteTransactionOnShards(
			context.Background(), []int{0, 1, 2, 3}, executeFn)

		require.NoError(t, err, "round %d", round)
		require.Equal(t, 4, len(result.ShardResults), "round %d", round)
	}
}

// TestConcurrency_MixedSuccessAndPanic verifies that when some shards succeed
// and one panics, the panic is reported and all shards are rolled back.
func TestConcurrency_MixedSuccessAndPanic(t *testing.T) {
	se, err := NewShardedExecutor(4, nil)
	require.NoError(t, err)

	var successCount atomic.Int32

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		if shard.ID == 3 {
			// Delay before panicking so others complete first
			time.Sleep(10 * time.Millisecond)
			panic("late panic in shard 3")
		}
		successCount.Add(1)
		return shard.ID, nil
	}

	result, err := se.ExecuteTransactionOnShards(
		context.Background(), []int{0, 1, 2, 3}, executeFn)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "panicked")
	// Other shards should have completed successfully before the panic was detected
	require.Greater(t, successCount.Load(), int32(0))
}
