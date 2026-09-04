// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestRepropose_UncommittedBatchReturnsUntilPruned pins the Narwhal batch
// delivery guarantee that run 20260820T090939Z proved missing: a batch this
// worker created must be proposed again until it commits. PruneBatches is the
// commit signal; a batch still stored reproposeAfter after it was last queued
// goes back on the availability queue, and a pruned batch stops.
func TestRepropose_UncommittedBatchReturnsUntilPruned(t *testing.T) {
	w := New(Config{
		ID:             0,
		Partition:      "test",
		BatchSize:      1,
		BatchTimeout:   10 * time.Millisecond,
		ReproposeAfter: 100 * time.Millisecond,
		ReproposeTick:  25 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Start(ctx) }()

	// Submit a transaction; the batch loop turns it into an own batch and
	// enqueues its digest.
	require.NoError(t, w.Submit([]byte("must-not-be-lost")))

	var digest types.BatchDigest
	require.Eventually(t, func() bool {
		ds := w.ConsumeAvailableBatches()
		if len(ds) == 0 {
			return false
		}
		digest = ds[0]
		return true
	}, 2*time.Second, 10*time.Millisecond, "batch never became available")

	// The digest was consumed (as if into a header) and its certificate never
	// commits. It must come back.
	require.Eventually(t, func() bool {
		for _, d := range w.ConsumeAvailableBatches() {
			if d == digest {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond,
		"an uncommitted batch must be re-proposed")

	// Commit it: prune removes it from the store, and re-proposal stops.
	w.PruneBatches([]types.BatchDigest{digest})
	time.Sleep(3 * (w.config.ReproposeAfter + w.config.ReproposeTick))
	for _, d := range w.ConsumeAvailableBatches() {
		require.NotEqual(t, digest, d, "a committed (pruned) batch must not be re-proposed")
	}
}

// TestRepropose_ForeignBatchesExcluded: batches received via gossip are the
// author's responsibility — re-proposing them from every holder would flood
// consensus with duplicates.
func TestRepropose_ForeignBatchesExcluded(t *testing.T) {
	w := New(Config{
		ID:             0,
		Partition:      "test",
		ReproposeAfter: 50 * time.Millisecond,
		ReproposeTick:  20 * time.Millisecond,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Start(ctx) }()

	// StoreBatch is the gossip ingest path — a foreign batch.
	foreign := types.NewBatch([][]byte{[]byte("someone-else's")})
	require.NoError(t, w.StoreBatch(foreign))

	time.Sleep(4 * (w.config.ReproposeAfter + w.config.ReproposeTick))
	for _, d := range w.ConsumeAvailableBatches() {
		require.NotEqual(t, foreign.Digest(), d, "foreign batches must not be re-proposed")
	}
}

// TestRequeueBatches_StampsLastQueued: an explicit requeue (uncertified-header
// path) must reset the re-proposal clock, or the same digest gets double-fed.
func TestRequeueBatches_StampsLastQueued(t *testing.T) {
	w := New(Config{ID: 0, Partition: "test"}, nil)

	batch := types.NewBatch([][]byte{[]byte("tx")})
	require.NoError(t, w.StoreBatch(batch))

	before := time.Now()
	w.RequeueBatches([]types.BatchDigest{batch.Digest()})

	w.batchMu.Lock()
	entry := w.batches[batch.Digest()]
	w.batchMu.Unlock()
	require.NotNil(t, entry)
	require.False(t, entry.lastQueued.Before(before),
		"RequeueBatches must stamp lastQueued so the re-proposal loop does not immediately double-feed the digest")

	// And the digest is actually available again.
	require.Equal(t, []types.BatchDigest{batch.Digest()}, w.ConsumeAvailableBatches())
}

// A batch a certified header names is never re-proposed, whatever its age
// (consensus spec, invariant 7). Re-proposing it put one batch in two
// certificates, and the second could not be served once the first had
// retired it (run 20260903T222843Z, C5, #4210).
func TestRepropose_SkipsBatchesTheDAGHasCertified(t *testing.T) {
	certified := map[types.BatchDigest]bool{}
	w := New(Config{
		ID: 0, Partition: "test", ReproposeAfter: time.Second,
		Certified: func(d types.BatchDigest) bool { return certified[d] },
	}, nil)

	a := types.NewBatch([][]byte{[]byte("in a certified header")})
	b := types.NewBatch([][]byte{[]byte("still waiting")})
	w.storeOwn(a)
	w.storeOwn(b)
	certified[a.Digest()] = true

	long := time.Now().Add(time.Hour) // both are far past ReproposeAfter
	stale, _ := w.staleOwnBatches(long)
	require.Equal(t, []types.BatchDigest{b.Digest()}, stale,
		"only the batch no certificate names is re-proposed")
}
