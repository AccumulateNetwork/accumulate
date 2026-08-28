// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// DefaultMaxInboundBatchBytes bounds the inbound batch queue in BYTES.
//
// The queue in front of the worker's batch store was a count cap of 1000 on
// items of up to MaxBatchBytes (500KiB) — ~500MB per partition, ~1GB per dual
// container, and unbounded in bytes for any batch size. That is the same bug
// class the worker's own store already fixed by byte-capping (5909219dc); the
// queue feeding it was never converted.
//
// 32MB matches DefaultMaxStoredBatchBytes: there is no point buffering more
// inbound batches than the store that receives them can hold.
const DefaultMaxInboundBatchBytes = 32 << 20

// batchQueue is a byte-bounded FIFO of inbound batches.
//
// Bounding by bytes rather than count is the point: a batch carries up to
// MaxBatchBytes, so a count cap says nothing about memory. Received batches
// alias the pubsub wire buffer (types.UnmarshalBatch takes ownership of
// msg.Data), so every queued batch pins its whole message allocation — the
// bytes held here are real resident memory, not an accounting fiction.
//
// When the queue is full the NEWEST batch is dropped, matching the previous
// full-channel behaviour: batches are re-broadcast by their author while
// uncommitted, so a drop costs a round trip, not the data.
type batchQueue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	items    []*types.Batch
	bytes    int
	maxBytes int
	closed   bool

	dropped atomic.Uint64 // batches refused because the queue was full
}

func newBatchQueue(maxBytes int) *batchQueue {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxInboundBatchBytes
	}
	q := &batchQueue{maxBytes: maxBytes}
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

// push enqueues a batch, reporting whether it was accepted. A batch larger
// than the whole budget is always refused rather than being allowed to sit
// alone in an over-budget queue.
func (q *batchQueue) push(b *types.Batch) bool {
	size := b.Size()

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed || q.bytes+size > q.maxBytes {
		q.dropped.Add(1)
		return false
	}

	q.items = append(q.items, b)
	q.bytes += size
	q.notEmpty.Signal()
	return true
}

// pop blocks until a batch is available or the queue is closed. The second
// return is false once the queue is closed and drained.
func (q *batchQueue) pop() (*types.Batch, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if len(q.items) == 0 {
		return nil, false
	}

	b := q.items[0]
	// Clear the slot so the batch (and the pubsub buffer it pins) can be
	// collected even while the backing array is still live.
	q.items[0] = nil
	q.items = q.items[1:]
	q.bytes -= b.Size()
	return b, true
}

// close wakes every waiter. Queued batches are discarded.
func (q *batchQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.items = nil
	q.bytes = 0
	q.notEmpty.Broadcast()
}

// stats reports queued bytes and the cumulative drop count.
func (q *batchQueue) stats() (queuedBytes int, dropped uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.bytes, q.dropped.Load()
}
