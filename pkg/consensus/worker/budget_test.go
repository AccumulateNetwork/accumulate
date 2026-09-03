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

// A full store is a state: it is logged when it changes and counted while it
// holds (invariant 5), not once per submission.
func TestOverLimit_IsLoggedOnTransition(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)

	var digests []types.BatchDigest
	for i := 0; i < 3; i++ {
		digests = append(digests, ownBatch(t, w, 4*1024, byte(i)))
	}
	for i := 0; i < 5; i++ {
		w.performEviction() // over limit, un-evictable: one transition, not five
	}
	require.Equal(t, uint64(1), w.overLimitChanges.Load())

	w.PruneCommitted(digests, CommitInfo{Detail: "test"})
	w.performEviction()
	require.Equal(t, uint64(2), w.overLimitChanges.Load(), "leaving the state is the second transition")
}

// Internal traffic -- synthetics, anchors, the healer's re-submissions -- is
// never refused: it is what drains the store (#4165). Only user submissions
// from the API are bounded.
func TestSubmit_InternalTrafficIsNeverRefused(t *testing.T) {
	w := New(Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 8 * 1024}, nil)
	for i := 0; i < 3; i++ {
		ownBatch(t, w, 4*1024, byte(i))
	}
	require.ErrorIs(t, w.SubmitUser([]byte{1}), ErrStoreFull)
	require.NoError(t, w.Submit([]byte{1}), "the internal path accepts while the user path refuses")
}
