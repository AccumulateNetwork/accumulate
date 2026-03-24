// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestWorker_LRUEviction verifies that LRU eviction policy is working correctly.
// It ensures that least recently used batches are evicted when storage limit is reached.
func TestWorker_LRUEviction(t *testing.T) {
	w := worker.New(worker.Config{
		ID:               0,
		Partition:        "test",
		MaxStoredBatches: 10, // Small limit to trigger eviction
	}, nil)

	// Store 10 batches (fill to capacity)
	var batches []*types.Batch
	var digests []types.BatchDigest
	for i := 0; i < 10; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("tx-%d", i))})
		batches = append(batches, batch)
		digests = append(digests, batch.Digest())
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	// Verify all batches are stored
	assert.Equal(t, 10, w.BatchCount())
	for i := 0; i < 10; i++ {
		assert.True(t, w.HasBatch(digests[i]), "batch %d should exist", i)
	}

	// Access batches 5-9 (making them recently used)
	// This leaves batches 0-4 as least recently used
	for i := 5; i < 10; i++ {
		retrieved, err := w.GetBatch(digests[i])
		require.NoError(t, err)
		require.NotNil(t, retrieved)
	}

	// Store one more batch, triggering eviction of 1 batch (10% of 10)
	newBatch := types.NewBatch([][]byte{[]byte("tx-new")})
	newDigest := newBatch.Digest()
	err := w.StoreBatch(newBatch)
	require.NoError(t, err)

	// Should still have 10 batches (evicted 1, added 1)
	assert.Equal(t, 10, w.BatchCount())

	// The new batch should exist
	assert.True(t, w.HasBatch(newDigest), "new batch should exist")

	// Batches 5-9 should still exist (they were recently accessed)
	for i := 5; i < 10; i++ {
		assert.True(t, w.HasBatch(digests[i]), "recently accessed batch %d should exist", i)
	}

	// At least one of batches 0-4 should have been evicted (LRU)
	survivingOldBatches := 0
	for i := 0; i < 5; i++ {
		if w.HasBatch(digests[i]) {
			survivingOldBatches++
		}
	}
	assert.Less(t, survivingOldBatches, 5, "some old batches should have been evicted")
}

// TestWorker_LRUEvictionOrder verifies that batches are evicted in LRU order.
func TestWorker_LRUEvictionOrder(t *testing.T) {
	w := worker.New(worker.Config{
		ID:               0,
		Partition:        "test",
		MaxStoredBatches: 5, // Very small limit for clear testing
	}, nil)

	// Store 5 batches
	var digests []types.BatchDigest
	for i := 0; i < 5; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("batch-%d", i))})
		digests = append(digests, batch.Digest())
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, w.BatchCount())

	// Access batch 0 (making it most recently used)
	_, err := w.GetBatch(digests[0])
	require.NoError(t, err)

	// Store a new batch, should evict batch 1 (now the LRU)
	newBatch := types.NewBatch([][]byte{[]byte("new-batch")})
	err = w.StoreBatch(newBatch)
	require.NoError(t, err)

	// Batch 0 should still exist (was accessed recently)
	assert.True(t, w.HasBatch(digests[0]), "batch 0 should still exist (recently accessed)")

	// At least one old batch should be gone due to eviction
	remainingOld := 0
	for i := 0; i < 5; i++ {
		if w.HasBatch(digests[i]) {
			remainingOld++
		}
	}
	assert.Less(t, remainingOld, 5, "at least one old batch should be evicted")
}

// TestWorker_LRUIdempotentStore verifies that storing the same batch twice
// updates its position in the LRU list without increasing storage.
func TestWorker_LRUIdempotentStore(t *testing.T) {
	w := worker.New(worker.Config{
		ID:               0,
		Partition:        "test",
		MaxStoredBatches: 5,
	}, nil)

	// Create and store a batch
	batch1 := types.NewBatch([][]byte{[]byte("batch-1")})
	digest1 := batch1.Digest()
	err := w.StoreBatch(batch1)
	require.NoError(t, err)

	// Store 4 more batches
	for i := 2; i <= 5; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("batch-%d", i))})
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, w.BatchCount())

	// Store batch1 again (should be idempotent and move it to front)
	err = w.StoreBatch(batch1)
	require.NoError(t, err)

	// Should still have exactly 5 batches (not 6)
	assert.Equal(t, 5, w.BatchCount())

	// batch1 should still exist
	assert.True(t, w.HasBatch(digest1))

	// Store a new batch, should evict the LRU (not batch1)
	newBatch := types.NewBatch([][]byte{[]byte("new-batch")})
	err = w.StoreBatch(newBatch)
	require.NoError(t, err)

	// batch1 should still exist (was refreshed)
	assert.True(t, w.HasBatch(digest1), "batch1 should still exist after refresh")
}

// TestWorker_LRUPruneRemovesFromList verifies that pruning removes batches
// from both the map and the LRU list.
func TestWorker_LRUPruneRemovesFromList(t *testing.T) {
	w := worker.New(worker.Config{
		ID:               0,
		Partition:        "test",
		MaxStoredBatches: 10,
	}, nil)

	// Store several batches
	var digests []types.BatchDigest
	for i := 0; i < 5; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("batch-%d", i))})
		digests = append(digests, batch.Digest())
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, w.BatchCount())

	// Prune first 3 batches
	w.PruneBatches(digests[:3])

	// Should have 2 batches remaining
	assert.Equal(t, 2, w.BatchCount())

	// Pruned batches should not exist
	for i := 0; i < 3; i++ {
		assert.False(t, w.HasBatch(digests[i]), "pruned batch %d should not exist", i)
	}

	// Remaining batches should exist
	for i := 3; i < 5; i++ {
		assert.True(t, w.HasBatch(digests[i]), "unpruned batch %d should exist", i)
	}

	// Add more batches to verify LRU list is still working after prune
	for i := 5; i < 10; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("batch-%d", i))})
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	// Should now have 7 batches (2 remaining + 5 new)
	assert.Equal(t, 7, w.BatchCount())
}

// TestWorker_LRUMassiveEviction verifies LRU works correctly with large eviction counts.
func TestWorker_LRUMassiveEviction(t *testing.T) {
	w := worker.New(worker.Config{
		ID:               0,
		Partition:        "test",
		MaxStoredBatches: 100,
	}, nil)

	// Fill to capacity
	var digests []types.BatchDigest
	for i := 0; i < 100; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("batch-%d", i))})
		digests = append(digests, batch.Digest())
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 100, w.BatchCount())

	// Access the last 50 batches (making them recently used)
	for i := 50; i < 100; i++ {
		_, err := w.GetBatch(digests[i])
		require.NoError(t, err)
	}

	// Store 20 more batches, triggering eviction of 10% = 10 batches each time
	for i := 100; i < 120; i++ {
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("batch-%d", i))})
		err := w.StoreBatch(batch)
		require.NoError(t, err)
	}

	// Should have 100 batches (evicted 20, added 20)
	assert.Equal(t, 100, w.BatchCount())

	// Recently accessed batches (50-99) should have higher survival rate
	recentlyAccessedSurvivors := 0
	for i := 50; i < 100; i++ {
		if w.HasBatch(digests[i]) {
			recentlyAccessedSurvivors++
		}
	}

	// Old batches (0-49) should have lower survival rate
	oldBatchSurvivors := 0
	for i := 0; i < 50; i++ {
		if w.HasBatch(digests[i]) {
			oldBatchSurvivors++
		}
	}

	// LRU should prefer keeping recently accessed over old batches
	// This is the key property we're testing
	assert.Greater(t, recentlyAccessedSurvivors, oldBatchSurvivors,
		"recently accessed batches should have higher survival rate than old batches")
}
