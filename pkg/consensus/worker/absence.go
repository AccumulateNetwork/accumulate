// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Why a batch is no longer in a worker's store.
//
// A committed certificate names batches, and the executor blocks until it has
// every one of them (consensus.CollectBatches). On 2026-08-21 the Directory
// halted permanently because one batch of the round-246 certificate was absent
// from all twelve validators: 190,500 identical "missing=1" warnings, and no
// way to tell whether the batch had been pruned, evicted, or never stored.
// Diagnosing it took a goroutine dump and two hours of log archaeology, and the
// answer was still a guess (#4125).
//
// Only two code paths remove a stored batch — PruneBatches and performEviction
// — so a tombstone at each one turns that question into a log line. The store
// keeps a bounded number of the most recent tombstones: enough to explain a
// wedge as it happens, fixed in memory.
const (
	// GoneePruned: removed because a certificate containing it was executed.
	// If this is why a LATER certificate cannot find it, the same digest
	// reached two certificates and the first commit deleted what the second
	// still needed.
	GonePruned = "pruned-after-commit"

	// GoneEvicted: dropped by LRU because the store exceeded MaxStoredBatches.
	GoneEvicted = "evicted-lru"

	// GoneUnknown: no tombstone. Either never stored on this node, or removed
	// longer ago than the tombstone ring retains.
	GoneUnknown = "no-record"
)

// DefaultMaxTombstones bounds the tombstone ring. Batches are ~KB; a tombstone
// is a digest, a short string and a timestamp, so this costs well under a MiB
// and covers far more than the handful of rounds a stall spans.
const DefaultMaxTombstones = 8192

// BatchGone records the removal of a batch from the store.
type BatchGone struct {
	Digest types.BatchDigest
	Reason string
	When   time.Time
	Detail string
}

// String renders a tombstone for a log line.
func (g BatchGone) String() string {
	s := g.Reason
	if g.Detail != "" {
		s += " (" + g.Detail + ")"
	}
	return fmt.Sprintf("%s %s ago", s, time.Since(g.When).Round(time.Millisecond))
}

// noteGone records why a digest left the store. The caller must hold batchMu.
//
// Re-recording a digest refreshes it rather than adding a second ring entry, so
// a batch that is stored and removed repeatedly cannot crowd out every other
// tombstone.
func (w *Worker) noteGone(digest types.BatchDigest, reason, detail string) {
	if w.maxTombstones <= 0 {
		return
	}
	if w.gone == nil {
		w.gone = make(map[types.BatchDigest]BatchGone)
	}
	if _, exists := w.gone[digest]; !exists {
		w.goneOrder = append(w.goneOrder, digest)
		// Trim oldest-first once over the limit.
		for len(w.goneOrder) > w.maxTombstones {
			oldest := w.goneOrder[0]
			w.goneOrder = w.goneOrder[1:]
			delete(w.gone, oldest)
		}
	}
	w.gone[digest] = BatchGone{Digest: digest, Reason: reason, When: time.Now(), Detail: detail}
}

// BatchGone reports why a digest is absent, if this worker knows.
// Returns ok=false when there is no tombstone for it.
func (w *Worker) BatchGone(digest types.BatchDigest) (BatchGone, bool) {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	g, ok := w.gone[digest]
	return g, ok
}

// TombstoneCount returns how many removals are currently remembered.
func (w *Worker) TombstoneCount() int {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	return len(w.gone)
}
