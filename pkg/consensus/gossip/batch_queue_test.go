// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// batchOfSize builds a batch holding one transaction of roughly n bytes.
func batchOfSize(n int) *types.Batch {
	return types.NewBatch([][]byte{make([]byte, n)})
}

// TestBatchQueueBoundsBytes is the point of the type: the old channel was a
// count cap on variable-size items, so 1000 slots x 500KiB was ~500MB.
func TestBatchQueueBoundsBytes(t *testing.T) {
	const budget = 1 << 20
	q := newBatchQueue(budget)

	// Push far more than the budget in small batches.
	accepted := 0
	for i := 0; i < 10_000; i++ {
		if q.push(batchOfSize(4096)) {
			accepted++
		}
	}

	queued, dropped := q.stats()
	require.LessOrEqual(t, queued, budget, "queue must never exceed its byte budget")
	require.Greater(t, accepted, 0, "some batches must be accepted")
	require.Equal(t, uint64(10_000-accepted), dropped)
}

// TestBatchQueueRejectsOversized pins that a single batch larger than the whole
// budget is refused rather than parked alone in an over-budget queue.
func TestBatchQueueRejectsOversized(t *testing.T) {
	q := newBatchQueue(1 << 20)
	require.False(t, q.push(batchOfSize(4<<20)))

	queued, _ := q.stats()
	require.Zero(t, queued)
}

// TestBatchQueueFreesBytesOnPop pins that popping restores budget — otherwise
// the queue would wedge shut after the first burst.
func TestBatchQueueFreesBytesOnPop(t *testing.T) {
	const budget = 64 << 10
	q := newBatchQueue(budget)

	require.True(t, q.push(batchOfSize(32<<10)))
	full, _ := q.stats()
	require.Greater(t, full, 0)

	_, ok := q.pop()
	require.True(t, ok)

	drained, _ := q.stats()
	require.Zero(t, drained, "popping must release the batch's bytes")

	// Budget is available again.
	require.True(t, q.push(batchOfSize(32<<10)))
}

// TestBatchQueueFIFO pins ordering: batches must not be reordered, since the
// worker treats earlier batches as available sooner.
func TestBatchQueueFIFO(t *testing.T) {
	q := newBatchQueue(1 << 20)
	sizes := []int{100, 200, 300}
	for _, n := range sizes {
		require.True(t, q.push(batchOfSize(n)))
	}
	for _, n := range sizes {
		b, ok := q.pop()
		require.True(t, ok)
		require.Equal(t, 1, b.Len())
		require.Len(t, b.Transactions[0], n, "batches must pop in push order")
	}
}

// TestBatchQueueCloseWakesPop pins that Close does not deadlock a blocked
// consumer — the pump goroutine parks in pop(), and cancelling the gossip
// context alone would never wake it.
func TestBatchQueueCloseWakesPop(t *testing.T) {
	q := newBatchQueue(1 << 20)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, ok := q.pop()
		require.False(t, ok, "pop must report closed")
	}()

	q.close()
	wg.Wait() // hangs if close does not broadcast
}
