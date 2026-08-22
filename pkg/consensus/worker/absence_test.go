// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

func batchOf(s string) *types.Batch {
	return types.NewBatch([][]byte{[]byte(s)})
}

// A batch removed because its certificate was executed says so, and names the
// block that did it.
//
// This is the diagnostic #4125 needed and did not have: the Directory halted
// on a batch that was absent from all twelve validators, and the log could not
// say whether it had been pruned, evicted, or never stored.
func TestPruneRecordsWhyTheBatchIsGone(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)

	b := batchOf("tx-1")
	require.NoError(t, w.StoreBatch(b))
	require.True(t, w.HasBatch(b.Digest()))

	_, ok := w.BatchGone(b.Digest())
	require.False(t, ok, "a stored batch must not have a tombstone")

	w.PruneBatchesAt([]types.BatchDigest{b.Digest()}, "block 42 round 246")

	require.False(t, w.HasBatch(b.Digest()))
	gone, ok := w.BatchGone(b.Digest())
	require.True(t, ok, "a pruned batch must leave a tombstone")
	assert.Equal(t, worker.GonePruned, gone.Reason)
	assert.Equal(t, "block 42 round 246", gone.Detail)
	assert.Contains(t, gone.String(), "pruned-after-commit")
	assert.Contains(t, gone.String(), "block 42 round 246")
}

// LRU eviction is a different cause and must not be confused with pruning:
// pruning means some certificate already committed the batch, eviction means
// the store was too small. Fixing the wrong one costs a night.
func TestEvictionRecordsADifferentCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := worker.New(worker.Config{
		ID:               0,
		Partition:        "test",
		MaxStoredBatches: 4,
	}, nil)
	go func() { _ = w.Start(ctx) }()
	defer w.Close()

	var first types.BatchDigest
	for i := 0; i < 40; i++ {
		b := batchOf(fmt.Sprintf("tx-%d", i))
		if i == 0 {
			first = b.Digest()
		}
		require.NoError(t, w.StoreBatch(b))
	}

	// Eviction runs on its own goroutine; wait for the oldest to go.
	require.Eventually(t, func() bool { return !w.HasBatch(first) },
		2*time.Second, 10*time.Millisecond, "the oldest batch should be evicted")

	gone, ok := w.BatchGone(first)
	require.True(t, ok, "an evicted batch must leave a tombstone")
	assert.Equal(t, worker.GoneEvicted, gone.Reason)
	assert.Contains(t, gone.Detail, "over limit 4")
}

// A batch this node simply never held is a third case, and reports as such
// rather than borrowing one of the other two explanations.
func TestNeverStoredHasNoTombstone(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)

	_, ok := w.BatchGone(batchOf("never-seen").Digest())
	assert.False(t, ok)
	assert.Equal(t, 0, w.TombstoneCount())
}

// The wedge shape from #4125, in miniature: one digest reaches two
// certificates, the first one to execute prunes it, and the second can never
// be collected. The point of the test is not that this sequence is legal — it
// is that when it happens the executor can say WHY, instead of repeating
// "missing=1" until someone takes a goroutine dump.
func TestSameBatchInTwoCertificates_SecondCannotCollectAndSaysWhy(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "Directory"}, nil)

	shared := batchOf("payment-1")
	digest := shared.Digest()
	require.NoError(t, w.StoreBatch(shared))

	// Certificate at round 240 commits and the executor prunes its payload.
	w.PruneBatchesAt([]types.BatchDigest{digest}, "block 2951 round 240")

	// Certificate at round 246 names the same digest. Collection fails.
	got, err := w.GetBatch(digest)
	require.NoError(t, err)
	require.Nil(t, got, "the second certificate cannot find its batch")

	// And the reason is available, naming the earlier block.
	gone, ok := w.BatchGone(digest)
	require.True(t, ok)
	assert.Equal(t, worker.GonePruned, gone.Reason)
	assert.Contains(t, gone.Detail, "round 240",
		"the tombstone must name the commit that deleted it, not the one that wanted it")
}

// The tombstone ring is bounded, so a long run cannot turn diagnostics into a
// memory leak. The oldest removals are forgotten first.
func TestTombstoneRingIsBounded(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test", MaxTombstones: 8}, nil)

	var digests []types.BatchDigest
	for i := 0; i < 50; i++ {
		b := batchOf(fmt.Sprintf("tx-%d", i))
		digests = append(digests, b.Digest())
		require.NoError(t, w.StoreBatch(b))
		w.PruneBatchesAt([]types.BatchDigest{b.Digest()}, fmt.Sprintf("block %d", i))
	}

	assert.LessOrEqual(t, w.TombstoneCount(), 8, "ring must stay bounded")

	_, ok := w.BatchGone(digests[0])
	assert.False(t, ok, "the oldest tombstone should have been forgotten")

	gone, ok := w.BatchGone(digests[49])
	require.True(t, ok, "the newest tombstone must be retained")
	assert.Equal(t, "block 49", gone.Detail)
}

// Re-storing and re-pruning the same digest refreshes its tombstone instead of
// consuming another ring slot — otherwise one churning batch would evict the
// record of every other.
func TestRepeatedRemovalRefreshesRatherThanGrows(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test", MaxTombstones: 4}, nil)

	b := batchOf("churn")
	other := batchOf("other")
	require.NoError(t, w.StoreBatch(other))
	w.PruneBatchesAt([]types.BatchDigest{other.Digest()}, "block 1")

	for i := 0; i < 20; i++ {
		require.NoError(t, w.StoreBatch(b))
		w.PruneBatchesAt([]types.BatchDigest{b.Digest()}, fmt.Sprintf("block %d", 100+i))
	}

	assert.Equal(t, 2, w.TombstoneCount(), "one churning digest occupies one slot")

	_, ok := w.BatchGone(other.Digest())
	assert.True(t, ok, "the churning batch must not have crowded out the other record")

	gone, _ := w.BatchGone(b.Digest())
	assert.Equal(t, "block 119", gone.Detail, "tombstone should hold the latest removal")
}

// Pruning a digest the worker never held must not invent a tombstone: absence
// of a record is itself information ("never stored here").
func TestPruningAnAbsentBatchRecordsNothing(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)

	w.PruneBatchesAt([]types.BatchDigest{batchOf("not-here").Digest()}, "block 7")
	assert.Equal(t, 0, w.TombstoneCount())
}

// The record can be turned off for a deployment that does not want it.
func TestTombstonesCanBeDisabled(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test", MaxTombstones: -1}, nil)

	b := batchOf("tx")
	require.NoError(t, w.StoreBatch(b))
	w.PruneBatchesAt([]types.BatchDigest{b.Digest()}, "block 1")

	assert.False(t, w.HasBatch(b.Digest()), "pruning still works")
	_, ok := w.BatchGone(b.Digest())
	assert.False(t, ok)
	assert.Equal(t, 0, w.TombstoneCount())
}
