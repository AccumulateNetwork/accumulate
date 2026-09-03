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

// Two boundaries, not one.
//
// Eviction used to be the first response to a full store, and what it evicts
// is other nodes' uncommitted batches -- exactly what this node needs to vote.
// Pressure was relieved by destroying progress. The first boundary refuses to
// SEAL instead: transactions stay pending, the pending caps push back on
// submitters, and the store stops growing from our own side while commits
// catch up. Eviction is what happens past the second boundary.
func TestSeal_StopsBeforeEvictionDoes(t *testing.T) {
	w := &Worker{
		batches: map[types.BatchDigest]*lruEntry{},
		pins:    map[types.BatchDigest]int{},
		config:  Config{MaxStoredBatches: 100, SealHighWater: 0.75},
	}
	w.lruList = list.New()
	w.maxStoredBytes = 1 << 30

	// Below the seal mark: sealing proceeds.
	for i := 0; i < 74; i++ {
		pinTestStore(w, digest(byte(i)))
	}
	require.False(t, w.sealBlocked(), "74 of 100 is under the 75% seal mark")

	// At the seal mark: sealing stops, and eviction has NOT started -- the
	// store is nowhere near its own limit.
	pinTestStore(w, digest(75))
	require.True(t, w.sealBlocked(), "75 of 100 is at the seal mark")

	w.performEviction()
	require.Equal(t, 75, len(w.batches),
		"backpressure first: nothing is evicted while the store is under its limit")
}

// Backpressure has to lift when the store drains, or a partition that hit the
// mark once would stop sealing forever.
func TestSeal_ResumesWhenTheStoreDrains(t *testing.T) {
	w := &Worker{
		batches: map[types.BatchDigest]*lruEntry{},
		pins:    map[types.BatchDigest]int{},
		config:  Config{MaxStoredBatches: 10, SealHighWater: 0.75},
	}
	w.lruList = list.New()
	w.maxStoredBytes = 1 << 30

	for i := 0; i < 8; i++ {
		pinTestStore(w, digest(byte(i)))
	}
	require.True(t, w.sealBlocked())

	// A commit prunes: the store drops back under the mark.
	for i := 0; i < 4; i++ {
		d := digest(byte(i))
		w.lruList.Remove(w.batches[d].element)
		delete(w.batches, d)
	}
	require.False(t, w.sealBlocked(), "drained below the mark, so sealing resumes")
}
