// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// mExecRunStopped counts staged entries that ran and did not move their
// stream (executor spec, "What a stream logs"). Nothing does this
// legitimately: an anchor below its quorum is never offered (it is not
// runnable), and a synthetic entry staging offered as runnable must move
// its stream. Any non-zero count, under either label, is a defect. #4423
// froze a stream this way for good, with every number held and nothing
// logged.
var mExecRunStopped = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "exec",
	Name:      "run_stopped_total",
	Help:      "Staged entries that ran without moving their stream, by stream kind (synthetic, anchor) and reason (not-delivered, error)",
}, []string{"stream", "reason"})

// RunStoppedEvery is how often a stream stopped at the same number is logged
// again, in block time.
const RunStoppedEvery = time.Minute

// runStopLogState is the last `Stream stopped` line per stream. Under the
// executor because it outlives the block; written only from the block's
// serial run phase.
type runStopLogState struct {
	mu   sync.Mutex
	last map[string]runStopLogEntry
}

type runStopLogEntry struct {
	number uint64
	at     time.Time
}

// noteRunStopped records a staged entry that ran without moving its stream:
// counted every time, logged once per stream and number, and again no more
// often than RunStoppedEvery while the stream stays stopped there.
func (b *Block) noteRunStopped(s stream, number uint64, statuses []*protocol.TransactionStatus, err error) {
	kind, reason := "synthetic", "not-delivered"
	if s.kind == streamAnchor {
		kind = "anchor"
	}
	if err != nil {
		reason = "error"
	}
	mExecRunStopped.WithLabelValues(kind, reason).Inc()

	l := &b.Executor.runStopLog
	l.mu.Lock()
	k := s.key()
	prev, seen := l.last[k]
	if seen && prev.number == number && b.Time.Sub(prev.at) < RunStoppedEvery {
		l.mu.Unlock()
		return
	}
	if l.last == nil {
		l.last = map[string]runStopLogEntry{}
	}
	l.last[k] = runStopLogEntry{number: number, at: b.Time}
	l.mu.Unlock()

	// What the entry's execution said about itself: for #4423 it was
	// "delivered", which is the whole diagnosis
	var said []string
	for _, st := range statuses {
		if st == nil {
			continue
		}
		if st.Error != nil {
			said = append(said, st.Code.String()+": "+st.Error.Message)
		} else {
			said = append(said, st.Code.String())
		}
	}
	kv := []any{"block", b.Index, "ledger", kind, "source", s.source, "number", number,
		"reason", reason, "status", strings.Join(said, "; "), "module", "stream"}
	if err != nil {
		kv = append(kv, "error", err)
	}
	b.Executor.logger.Info("Stream stopped", kv...)
}
