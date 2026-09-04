// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// The batch plane's memory is bounded in bytes, and own uncommitted batches
// are bounded by refusing work, not by growing (consensus spec, invariants
// 1 and 4). Runs 20260903T173742Z, 202621Z and 213153Z all ended with own
// uncommitted batches past every limit while the worker went on accepting.

func ownBatch(t *testing.T, w *Worker, size int, tag byte) types.BatchDigest {
	t.Helper()
	tx := make([]byte, size)
	tx[0] = tag
	b := types.NewBatch([][]byte{tx})
	w.storeOwn(b)
	return b.Digest()
}

func TestSubmit_RefusesWhenOwnBatchesFillTheBudget(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)

	var digests []types.BatchDigest
	for i := 0; i < 3; i++ { // 3 x 4 KB of own batches against an 8 KB share
		digests = append(digests, ownBatch(t, w, 4*1024, byte(i)))
	}

	err := w.SubmitUser([]byte{1, 2, 3})
	require.ErrorIs(t, err, ErrStoreFull, "own batches fill the share: the worker refuses, it does not grow")

	// Committing them frees the share and the worker accepts again.
	w.PruneCommitted(digests, CommitInfo{Detail: "test"})
	require.NoError(t, w.SubmitUser([]byte{1, 2, 3}))
}

func TestSubmit_PendingCountsAgainstTheBudget(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)

	// Nothing sealed (no batch loop is running): pending alone must be
	// able to fill the share, and then refuse.
	require.NoError(t, w.SubmitUser(make([]byte, 4*1024)))
	require.NoError(t, w.SubmitUser(make([]byte, 3*1024)))
	err := w.SubmitUser(make([]byte, 2*1024))
	require.ErrorIs(t, err, ErrStoreFull)
}

func TestEviction_IsByBytesNotByCount(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 4 << 20}, nil)

	// Far more batches than any count limit ever allowed, well under the
	// byte budget: none may be evicted. A count says nothing about memory.
	for i := 0; i < 5000; i++ {
		tx := make([]byte, 64)
		tx[0], tx[1] = byte(i), byte(i>>8)
		require.NoError(t, w.StoreBatch(types.NewBatch([][]byte{tx})))
	}
	w.performEviction()
	w.batchMu.Lock()
	n := len(w.batches)
	w.batchMu.Unlock()
	require.Equal(t, 5000, n)
}

// Refusal is a state: it is logged when it changes and counted while it
// holds (invariant 5), not once per submission.
func TestRefusal_IsLoggedOnTransition(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)

	var digests []types.BatchDigest
	for i := 0; i < 3; i++ {
		digests = append(digests, ownBatch(t, w, 4*1024, byte(i)))
	}
	for i := 0; i < 5; i++ {
		require.ErrorIs(t, w.SubmitUser([]byte{1}), ErrStoreFull) // five refusals, one transition
	}
	require.Equal(t, uint64(1), w.refusingChanges.Load())

	w.PruneCommitted(digests, CommitInfo{Detail: "test"})
	require.NoError(t, w.SubmitUser([]byte{1}))
	require.Equal(t, uint64(2), w.refusingChanges.Load(), "accepting again is the second transition")
}

// Own batches and the peer cache do not share a budget (invariant 8): a
// worker whose own batches exceed their share must still hold every peer
// batch, because those are what the next header's vote needs (invariant 3).
// Run 20260903T222843Z emptied the peer cache this way and stalled BVN2.
func TestPeerCache_SurvivesOwnOverflow(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)

	for i := 0; i < 4; i++ { // 16 KB of own batches against an 8 KB share
		ownBatch(t, w, 4*1024, byte(i))
	}
	var peers []types.BatchDigest
	for i := 0; i < 4; i++ { // 4 KB of peers' batches, within their own 8 KB budget
		tx := make([]byte, 1024)
		tx[0], tx[1] = 0xee, byte(i)
		b := types.NewBatch([][]byte{tx})
		require.NoError(t, w.StoreBatch(b))
		peers = append(peers, b.Digest())
	}
	w.performEviction()
	for _, d := range peers {
		require.True(t, w.HasBatch(d), "a peer batch within the peer budget survives own overflow")
	}
	require.ErrorIs(t, w.SubmitUser([]byte{1}), ErrStoreFull, "and the own overflow is what refuses")
}
