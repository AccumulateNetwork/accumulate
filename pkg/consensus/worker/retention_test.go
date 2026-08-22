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

// The point of retention: a peer that missed the commit can still be served.
//
// Before this, a batch's life on the network ended at the first commit that
// included it, simultaneously on every validator. Three of twelve validators in
// soak run 20260822T015342Z were restarted or paused for under three minutes,
// came back asking for early rounds, and never advanced again — zero hits
// across 55,000 peer requests, because there was nothing left anywhere (#4128).
func TestCommittedBatchStaysFetchableForPeers(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "BVN1"}, nil)

	b := batchOf("tx-1")
	require.NoError(t, w.StoreBatch(b))
	w.PruneCommitted([]types.BatchDigest{b.Digest()},
		worker.CommitInfo{Cert: "cert-a", Detail: "block 10"})

	got, err := w.GetBatch(b.Digest())
	require.NoError(t, err)
	require.NotNil(t, got, "a lagging peer must still be able to fetch this")
	assert.Equal(t, 1, w.RetainedCount())
}

// Retention must not resurrect a batch into the work the node still owes
// consensus. A committed batch that returned to the active store would be
// re-proposed forever, which is the loss #4122 fixed from the other direction.
func TestRetainedBatchIsNotActiveAndIsNotReproposed(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "BVN1"}, nil)

	b := batchOf("tx-1")
	require.NoError(t, w.StoreBatch(b))
	before := w.BatchCount()
	w.PruneCommitted([]types.BatchDigest{b.Digest()}, worker.CommitInfo{Cert: "cert-a"})

	assert.False(t, w.HasBatch(b.Digest()), "committed batches leave the active store")
	assert.Equal(t, before-1, w.BatchCount())
	assert.True(t, w.HasRetained(b.Digest()), "but are retained for peers")
}

// The window closes. Retention is a grace period, not a second database.
func TestRetentionExpiresAndSaysSo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1",
		RetainCommittedFor: 50 * time.Millisecond,
	}, nil)
	go func() { _ = w.Start(ctx) }()
	defer w.Close()

	b := batchOf("tx-1")
	require.NoError(t, w.StoreBatch(b))
	w.PruneCommitted([]types.BatchDigest{b.Digest()},
		worker.CommitInfo{Cert: "cert-a", Detail: "block 10 round 7"})
	require.True(t, w.HasRetained(b.Digest()))

	require.Eventually(t, func() bool { return !w.HasRetained(b.Digest()) },
		2*time.Second, 20*time.Millisecond, "the retention window should close")

	got, err := w.GetBatch(b.Digest())
	require.NoError(t, err)
	assert.Nil(t, got)

	gone, ok := w.BatchGone(b.Digest())
	require.True(t, ok)
	assert.Equal(t, worker.GoneRetentionExpired, gone.Reason,
		"an expiry is a different cause from a fresh commit, and must read that way")
	assert.Contains(t, gone.Detail, "block 10 round 7",
		"and still names the commit that retired it")
}

// Retention is bounded by count as well as age, so a burst cannot grow it
// without limit. A validator that cannot bound its memory dies of something
// else instead.
func TestRetentionIsBoundedByCount(t *testing.T) {
	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1",
		MaxRetainedBatches: 8,
		RetainCommittedFor: time.Hour, // age must not be what bounds this test
	}, nil)

	var digests []types.BatchDigest
	for i := 0; i < 50; i++ {
		b := batchOf(fmt.Sprintf("tx-%d", i))
		digests = append(digests, b.Digest())
		require.NoError(t, w.StoreBatch(b))
		w.PruneCommitted([]types.BatchDigest{b.Digest()},
			worker.CommitInfo{Cert: fmt.Sprintf("cert-%d", i)})
	}

	assert.LessOrEqual(t, w.RetainedCount(), 8)
	assert.False(t, w.HasRetained(digests[0]), "oldest retained batch is dropped first")
	assert.True(t, w.HasRetained(digests[49]), "newest is kept")

	gone, ok := w.BatchGone(digests[0])
	require.True(t, ok)
	assert.Equal(t, worker.GoneRetentionExpired, gone.Reason)
}

// Retention can be turned off, restoring delete-on-commit for a deployment
// that would rather have the memory than the recoverability.
func TestRetentionCanBeDisabled(t *testing.T) {
	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1", MaxRetainedBatches: -1,
	}, nil)

	b := batchOf("tx-1")
	require.NoError(t, w.StoreBatch(b))
	w.PruneCommitted([]types.BatchDigest{b.Digest()}, worker.CommitInfo{Cert: "cert-a"})

	assert.Equal(t, 0, w.RetainedCount())
	got, err := w.GetBatch(b.Digest())
	require.NoError(t, err)
	assert.Nil(t, got)
}
