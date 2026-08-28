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

// The byte cap is the memory governor: count caps alone let 728MB of gossiped
// batches accumulate per node instance — two instances per 4GiB cgroup — and
// the fleet was OOM-killed (#4164, runs 20260824T065208Z / 20260824T112437Z).
func TestWorker_ByteCapEvictsGossipCopies(t *testing.T) {
	w := New(Config{
		ID:                  1,
		Partition:           "test",
		MaxStoredBatches:    100000, // count cap out of the way
		MaxStoredBatchBytes: 64 * 1024,
	}, nil)

	// Store gossip copies (not own) well past the byte cap.
	for i := 0; i < 40; i++ {
		tx := make([]byte, 4*1024)
		tx[0] = byte(i)
		require.NoError(t, w.StoreBatch(types.NewBatch([][]byte{tx})))
	}
	w.performEviction()

	w.batchMu.Lock()
	stored, bytes := len(w.batches), w.storedBytes
	w.batchMu.Unlock()
	require.LessOrEqual(t, bytes, 64*1024, "the store must not exceed its byte cap after eviction")
	require.Less(t, stored, 40, "gossip copies beyond the byte cap are evicted")
}

// Own uncommitted batches are NEVER evicted, whatever the byte pressure —
// losing them wedges the partition (#4159). The cap must squeeze gossip
// copies only and say so loudly when it cannot reach its target.
func TestWorker_ByteCapNeverEvictsOwnUncommitted(t *testing.T) {
	w := New(Config{
		ID:                  1,
		Partition:           "test",
		MaxStoredBatches:    100000,
		MaxStoredBatchBytes: 8 * 1024, // tiny: every own batch exceeds it
	}, nil)

	for i := 0; i < 5; i++ {
		tx := make([]byte, 4*1024)
		tx[0] = byte(i)
		b := types.NewBatch([][]byte{tx})
		w.batchMu.Lock()
		element := w.lruList.PushFront(b.Digest())
		w.batches[b.Digest()] = &lruEntry{batch: b, element: element, own: true}
		w.storedBytes += batchBytes(b)
		w.batchMu.Unlock()
	}
	w.performEviction()

	w.batchMu.Lock()
	stored := len(w.batches)
	w.batchMu.Unlock()
	require.Equal(t, 5, stored, "own uncommitted batches survive byte pressure — evicting them wedges the partition")
}
