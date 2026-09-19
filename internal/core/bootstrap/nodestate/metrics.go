// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// WHY THESE LIVE HERE AND NOT IN THE JOIN
//
// accumulate_node_state used to be declared in internal/node/join, and a
// Prometheus GaugeVec creates a child the first time WithLabelValues names
// it — so a node that never entered the join state machine exported NO SUCH
// SERIES AT ALL. Measured on the twelve-node network of 2026-09-18, at one
// moment, with one query:
//
//	acc-bvn1-val1 (joining)  accumulate_node_state{partition="bvn1"} 0
//	                         accumulate_node_state{partition="directory"} 0
//	acc-bvn1-val2 (healthy)  (no accumulate_node_state series)
//
// "Absent" therefore meant "this process has not been in the join state
// machine this lifetime", which is neither ACTIVE nor unhealthy. A monitor
// reading absent as 0 paints a healthy fleet as booting, and one reading it
// as "fine" can never assert that a node is alive. The metric could only ever
// say something was wrong, and only while it was wrong (#4345a).
//
// They are reported by the daemon, for every partition the process runs, from
// start-up. The join is one of the callers, not the only one.

// mNodeState is the state this node is in for one partition.
var mNodeState = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "node",
	Name:      "state",
	Help:      "This node's state for the partition: 0 booting, 1 waiting, 2 active, 3 complete (executor spec, \"Sync\", step 5)",
}, []string{"partition"})

// mExecutedBlock is the block THIS node's executor last executed.
//
// It is the single number that tells a wedged node from a healthy one, and
// nothing exported it: accumulate_exec_blocks_total is process-wide with no
// partition label, so on a node running the Directory and a BVN it is one
// series summing two chains (5264 on acc-bvn1-val2), and it reads 0 on a node
// that is joining whatever that node has executed. Reading one node's height
// therefore cost two JSON-RPC calls per node per partition, which is why
// nothing did it per node (#4345b).
//
// A gauge, not a counter: it is a position, not a rate.
//
// It is the block the executor last RAN, which on an idle network is ahead of
// the block it last WROTE: an empty block discards its batch, so it advances
// no ledger and no durable record (block.go, closedBlock.Commit). That is the
// number a monitor wants — "is this node still executing" — and it is why an
// idle network reads as healthy here and as frozen on the ledger index. At
// start-up, before this process has run a block, it is seeded from the
// durable record, which is the last block this node WROTE.
var mExecutedBlock = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "node",
	Name:      "executed_block",
	Help:      "The block THIS node's executor last executed for the partition — its own height, never a routed answer (#4345)",
}, []string{"partition"})

// Number is the state as the gauge reports it: 0 booting, 1 waiting, 2
// active, 3 complete.
func Number(s State) float64 {
	switch s {
	case StateWaiting:
		return 1
	case StateActive:
		return 2
	case StateComplete:
		return 3
	default:
		return 0
	}
}

// label is the partition as the exposition names it. Lower case, because that
// is what accumulate_node_state has always exported and a monitor that
// matches on it must keep working.
//
// NOTE that accumulate_exec_* and accumulate_dagbft_* spell the same
// partition as its ID — "BVN1", "Directory" — so a PromQL join across the two
// families needs one side folded. That predates this file (compare
// accumulate_dagbft_execution_lag_blocks) and is not changed here: renaming
// an existing series' label values is a break, and the one consumer that
// reads accumulate_node_state canonicalises both spellings already
// (test/docker/soak/nodewatch.py, canon_part).
func label(partition string) string { return strings.ToLower(partition) }

// Report exports the state this node is in for one partition. Every node
// calls it, for every partition it runs, whether or not it ever joins.
func Report(partition string, s State) {
	mNodeState.WithLabelValues(label(partition)).Set(Number(s))
}

// ReportExecuted exports the block this node's own executor last executed for
// one partition.
func ReportExecuted(partition string, block uint64) {
	mExecutedBlock.WithLabelValues(label(partition)).Set(float64(block))
}
