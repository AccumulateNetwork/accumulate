// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// #4169 step 0 — the baseline that gates group 4 (sharded execution) and the
// optional two-round staging. Three questions, each answered by a counter the
// soak monitor reads off the node, so the answer comes from a 12h run and not
// from an argument:
//
//   0a. Serial share. How much of a block's execution wall time is spent in
//       the serial lane versus the parallel flushes. Sharding user
//       transactions at 93.7% across 8 shards did not move throughput; if
//       the serial share is below 25% there is nothing for group 4 to win.
//   0b. Flushes per block. How many parallel runs a block actually forms — a
//       block that flushes once per envelope is serial with extra steps.
//   0c. Anchor/synthetic co-arrival. How often a synthetic's proving anchor
//       was applied in the SAME block. Below 5%, the anchors-then-synthetics
//       round costs more than it saves.
//
// Counters rather than gauges, so a scrape at any interval can take a delta
// and the fleet total is a sum. Ratios are the monitor's to compute.

var mExecPhaseSeconds = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "exec",
	Name:      "phase_seconds_total",
	Help:      "Wall time spent in ProcessAll by phase: serial (staging, drains, barrier envelopes) or parallel (shard flushes, including their serial commit)",
}, []string{"phase"}) // serial | parallel

var mExecBlocks = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "exec",
	Name:      "blocks_total",
	Help:      "Blocks closed by the executor",
})

var mExecFlushes = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "exec",
	Name:      "flushes_total",
	Help:      "Parallel runs flushed (a run is consecutive single-identity user envelopes executed across shards)",
})

var mExecSyntheticAnchor = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "exec",
	Name:      "synthetic_anchor_total",
	Help:      "Synthetics judged by staging, by when their proving anchor was applied: this_block, earlier, or missing (not yet anchored)",
}, []string{"applied"}) // this_block | earlier | missing
