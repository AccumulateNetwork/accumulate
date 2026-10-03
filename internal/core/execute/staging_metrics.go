// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// What staging holds, per stream (#4233). Staging is bounded by execution,
// not by a budget: the collected set grows with the Directory's anchor
// latency times the inbound rate, so what an operator needs is to see it,
// and a line when it is more than a few blocks' worth. Labels are the stream
// key — ledger and source — and there is one stream per pair of partitions.
var mStagingHeld = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "staging",
	Name:      "held_entries",
	Help:      "Entries held in staging for the stream: received and not yet executed",
}, []string{"ledger", "source"})

var mStagingHeldBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "staging",
	Name:      "held_bytes",
	Help:      "Encoded size of the entries held in staging for the stream",
}, []string{"ledger", "source"})

// heldAlarmEntries is the backlog that is reported: eight blocks' worth at
// the most a block executes from one stream (the executor's maxRunPerBlock,
// 1024). Cleared once the backlog is below half of it, so a stream hovering
// at the line is reported once, not every block.
// mStreamDegraded is 1 while the stream's head has been held for an anchor
// below its signature quorum for DegradedAfterBlocks: the source partition is
// below its signing quorum (#4510).
var mStreamDegraded = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "staging",
	Name:      "stream_degraded",
	Help:      "1 while the stream's head has been held for an anchor below its signature quorum for a minute of blocks: its source partition is below its signing quorum",
}, []string{"ledger", "source"})

// mStagingRefused counts entries staging refused to hold, by reason.
var mStagingRefused = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "staging",
	Name:      "refused_total",
	Help:      "Entries staging refused to hold, by reason: degraded (the stream's source is below its quorum and the hold is at its bound)",
}, []string{"ledger", "source", "reason"})

const heldAlarmEntries = 8 * 1024

// heldSize is the message's encoded size, measured once when the entry is
// held.
func heldSize(h *Held) int {
	if h.Message == nil {
		return 0
	}
	b, err := h.Message.MarshalBinary()
	if err != nil {
		return 0
	}
	return len(b)
}

// observe publishes what the stream holds and reports a backlog once, when
// it crosses the line, and its clearing once.
func (st *streamState) observe(key string) {
	ledger, source, _ := strings.Cut(key, "|")
	mStagingHeld.WithLabelValues(ledger, source).Set(float64(st.held))
	mStagingHeldBytes.WithLabelValues(ledger, source).Set(float64(st.bytes))
	switch {
	case !st.degradedOn && st.degraded():
		st.degradedOn = true
		mStreamDegraded.WithLabelValues(ledger, source).Set(1)
		slog.Warn("A stream's source partition is below its signing quorum: its head has been held for an anchor below quorum for a minute of blocks; what is held for it is bounded",
			"module", "staging", "ledger", ledger, "source", source, "since-block", st.belowSince, "at-block", st.belowAt, "held", st.held, "limit", DegradedHoldLimit)
	case st.degradedOn && !st.degraded():
		st.degradedOn = false
		mStreamDegraded.WithLabelValues(ledger, source).Set(0)
		slog.Info("A stream's source partition is back at its signing quorum", "module", "staging", "ledger", ledger, "source", source)
	}
	switch {
	case !st.alarmed && st.held > heldAlarmEntries:
		st.alarmed = true
		slog.Info("Staging holds more than several blocks' worth for a stream: its proofs or anchors are late", "module", "staging", "ledger", ledger, "source", source, "held", st.held, "bytes", st.bytes)
	case st.alarmed && st.held <= heldAlarmEntries/2:
		st.alarmed = false
		slog.Info("Staging backlog cleared for a stream", "module", "staging", "ledger", ledger, "source", source, "held", st.held)
	}
}
