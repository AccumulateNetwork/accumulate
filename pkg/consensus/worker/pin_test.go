// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"container/list"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func digest(n byte) types.BatchDigest { var d types.BatchDigest; d[0] = n; return d }

// pinTestStore puts a gossiped (not authored) batch in the store, oldest
// first, without going through the batching path.
func pinTestStore(w *Worker, d types.BatchDigest) {
	e := &lruEntry{batch: &types.Batch{}, own: false}
	e.element = w.lruList.PushFront(d)
	w.batches[d] = e
}

// A batch a header is waiting on is not cache. Before pinning, the only
// protected batches were the ones this node authored -- which is the wrong
// half: a header names several batches, one is missing, the vote defers, and
// the ones we DO hold get evicted while we wait. The next rebroadcast then
// finds a different batch missing. Measured in soak 20260902T231641Z: 777
// fetches for 172 distinct batches, one asked 29 times, store turning over
// 1.7x/second, and the partition stopped producing blocks.
func TestPin_ProtectsBatchesAHeaderIsWaitingOn(t *testing.T) {
	w := &Worker{
		batches: map[types.BatchDigest]*lruEntry{},
		pins:    map[types.BatchDigest]int{},
		config:  Config{MaxStoredBatches: 2},
	}
	w.lruList = list.New()
	w.maxStoredBytes = 1 << 30 // count is the binding cap here

	// Three gossiped batches, none authored here. Least-recently-used first.
	for i := byte(1); i <= 3; i++ {
		pinTestStore(w, digest(i))
	}

	// A header names the two oldest and is waiting on something else.
	w.PinBatches([]types.BatchDigest{digest(1), digest(2)})
	require.Equal(t, 2, w.PinnedBatches())

	w.performEviction()

	require.True(t, w.HasBatch(digest(1)), "pinned: a vote is waiting on it")
	require.True(t, w.HasBatch(digest(2)), "pinned")
	require.False(t, w.HasBatch(digest(3)), "unpinned and newest, but nothing is waiting on it")
}

// Releasing is what keeps the pin set bounded. A pin never released is a batch
// that can never be evicted, which is the memory bound failing open.
func TestPin_ReleaseRestoresEvictability(t *testing.T) {
	w := &Worker{
		batches: map[types.BatchDigest]*lruEntry{},
		pins:    map[types.BatchDigest]int{},
		config:  Config{MaxStoredBatches: 1},
	}
	w.lruList = list.New()
	w.maxStoredBytes = 1 << 30

	pinTestStore(w, digest(1))
	pinTestStore(w, digest(2))

	w.PinBatches([]types.BatchDigest{digest(1)})
	w.performEviction()
	require.True(t, w.HasBatch(digest(1)))

	w.UnpinBatches([]types.BatchDigest{digest(1)})
	require.Zero(t, w.PinnedBatches(), "released, so it stops holding memory hostage")

	// Put the store back over its limit: only then does eviction run at all.
	pinTestStore(w, digest(3))
	w.performEviction()
	require.False(t, w.HasBatch(digest(1)), "evictable again once nothing waits on it")
}

// Two headers can name the same batch. It must stay pinned until BOTH are
// done, or the second header loses the batch the first released.
func TestPin_NestsAcrossHeaders(t *testing.T) {
	w := &Worker{pins: map[types.BatchDigest]int{}}

	w.PinBatches([]types.BatchDigest{digest(1)})
	w.PinBatches([]types.BatchDigest{digest(1)})
	w.UnpinBatches([]types.BatchDigest{digest(1)})
	require.Equal(t, 1, w.PinnedBatches(), "one header is still waiting")

	w.UnpinBatches([]types.BatchDigest{digest(1)})
	require.Zero(t, w.PinnedBatches())
}

// A pin on a batch we do not hold is meaningful: it says "when this arrives,
// keep it". It must not resurrect or fabricate an entry.
func TestPin_OnAbsentBatchIsHarmless(t *testing.T) {
	w := &Worker{batches: map[types.BatchDigest]*lruEntry{}, pins: map[types.BatchDigest]int{}}
	w.PinBatches([]types.BatchDigest{digest(9)})
	require.False(t, w.HasBatch(digest(9)))
	require.Equal(t, 1, w.PinnedBatches())
}
