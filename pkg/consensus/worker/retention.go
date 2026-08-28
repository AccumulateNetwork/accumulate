// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Batches are kept for a while after they commit, so a node that fell behind
// can still fetch them.
//
// Before this, PruneBatches deleted a batch the moment its certificate was
// executed, on every validator at once — so a batch's lifetime on the network
// was "until the first commit that includes it". A node that missed that
// window had no source anywhere: in soak run 20260822T015342Z three of twelve
// validators were restarted or paused for under three minutes, came back
// asking for round 1, and never advanced again, with zero hits across 55,000
// peer requests (#4128). In production that is every upgrade, crash, OOM and
// long GC pause.
//
// Retention is a second-chance store, deliberately separate from the active
// batch map: the active map is what this node still owes to consensus, and
// retention is what it can still serve to someone else. Keeping them apart
// means re-proposal, eviction and the commit signal all keep their existing
// meaning — a retained batch is committed, so it is never re-proposed — while
// GetBatch, and through it the peer fetch handler, can still answer.
//
// It is bounded twice, by age and by count, because a validator that cannot
// bound its memory is a validator that dies of something else instead.
const (
	// DefaultRetainCommittedFor is how long a committed batch stays fetchable.
	// Long enough to cover a restart, a pause, or a slow catch-up; short
	// enough that it is a window and not a second database.
	DefaultRetainCommittedFor = 10 * time.Minute

	// DefaultMaxRetainedBatches caps the retention store independently of
	// MaxStoredBatches. Batches are ~KB, so this is tens of MB at worst.
	DefaultMaxRetainedBatches = 4096

	// GoneRetentionExpired: held after commit, then dropped when the window
	// closed. Distinct from pruned-after-commit, which now means "committed
	// and no longer retained here" only once this has happened.
	GoneRetentionExpired = "retention-expired"
)

// retainedBatch is a committed batch kept available for lagging peers.
type retainedBatch struct {
	batch *types.Batch
	at    time.Time
	// detail is the commit that retired it — the same string the tombstone
	// carries, so an expiry can still say which block committed the batch.
	detail string
	// cert is the certificate that committed it, carried so an expiry
	// tombstone can still recognise a re-delivery.
	cert string
}

// retain moves a committed batch into the retention store. The caller must
// hold batchMu.
func (w *Worker) retain(digest types.BatchDigest, b *types.Batch, detail, cert string) {
	if w.maxRetained <= 0 || b == nil {
		return
	}
	if w.retained == nil {
		w.retained = make(map[types.BatchDigest]*retainedBatch)
	}
	if prev, exists := w.retained[digest]; !exists {
		w.retainedOrder = append(w.retainedOrder, digest)
	} else {
		w.retainedBytes -= batchBytes(prev.batch)
	}
	w.retained[digest] = &retainedBatch{batch: b, at: time.Now(), detail: detail, cert: cert}
	w.retainedBytes += batchBytes(b)
	metrics.BatchesRetained.Set(float64(len(w.retained)))

	// Oldest first, so neither cap can be exceeded even in a burst. The byte
	// cap is what actually bounds memory (#4164).
	for len(w.retainedOrder) > 0 &&
		(len(w.retainedOrder) > w.maxRetained || w.retainedBytes > w.maxRetainedBytes) {
		oldest := w.retainedOrder[0]
		w.retainedOrder = w.retainedOrder[1:]
		if r, ok := w.retained[oldest]; ok {
			delete(w.retained, oldest)
			w.retainedBytes -= batchBytes(r.batch)
			w.noteGone(oldest, GoneRetentionExpired, r.detail, r.cert)
			metrics.BatchesRetentionExpiredTotal.Inc()
		}
	}
}

// getRetained returns a committed batch still inside the retention window.
func (w *Worker) getRetained(digest types.BatchDigest) *types.Batch {
	r, ok := w.retained[digest]
	if !ok {
		return nil
	}
	// A hit here is a peer that would otherwise have been stranded (#4128).
	metrics.BatchRetentionHitsTotal.Inc()
	return r.batch
}

// sweepRetained drops committed batches whose window has closed. Called from
// the eviction loop, which already runs on a timer.
func (w *Worker) sweepRetained() {
	if w.retainFor <= 0 {
		return
	}
	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	cutoff := time.Now().Add(-w.retainFor)
	dropped := 0
	// retainedOrder is append-ordered, so everything expired is at the front.
	for len(w.retainedOrder) > 0 {
		d := w.retainedOrder[0]
		r, ok := w.retained[d]
		if !ok {
			w.retainedOrder = w.retainedOrder[1:]
			continue
		}
		if r.at.After(cutoff) {
			break
		}
		w.retainedOrder = w.retainedOrder[1:]
		delete(w.retained, d)
		w.retainedBytes -= batchBytes(r.batch)
		w.noteGone(d, GoneRetentionExpired, r.detail, r.cert)
		metrics.BatchesRetentionExpiredTotal.Inc()
		dropped++
	}
	metrics.BatchesRetained.Set(float64(len(w.retained)))
	if dropped > 0 {
		slog.Debug("Retention window closed for committed batches",
			"dropped", dropped, "retained", len(w.retained),
			"worker", w.config.ID, "partition", w.config.Partition)
	}
}

// RetainedCount returns how many committed batches are still fetchable.
func (w *Worker) RetainedCount() int {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	return len(w.retained)
}

// HasRetained reports whether a committed batch is still being kept for peers.
func (w *Worker) HasRetained(digest types.BatchDigest) bool {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	_, ok := w.retained[digest]
	return ok
}
