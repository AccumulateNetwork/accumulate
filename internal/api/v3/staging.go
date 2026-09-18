// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// What this node has served of its staging (#4291). A joining node reads a
// peer's whole stage, which under load is thousands of entries, so what an
// operator needs to see is that it is happening and what it costs.
var (
	mStagingSnapshots = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "staging",
		Name:      "snapshots_total",
		Help:      "Pages of this node's staging served to a joining node",
	}, []string{"partition"})

	mStagingSnapshotBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "staging",
		Name:      "snapshot_bytes_total",
		Help:      "Encoded size of the staging pages this node has served",
	}, []string{"partition"})
)

var _ private.StagingSnapshotter = (*Sequencer)(nil)

// stagingFor is the staging this sequencer serves: the one it was given, or
// the one registered for its partition. A process running several networks
// (the simulator) has no registry to go to and hands it over directly.
func (s *Sequencer) stagingFor() *execute.Staging {
	if s.staging != nil {
		return s.staging
	}
	return execute.StagingFor(s.partitionID)
}

// StagingSnapshot serves this node's staging as of its last committed block:
// everything above Delivered it holds, unexecuted, so a node that joins or
// restarts starts from what its peers hold rather than from what a source
// produced (executor spec, "Sync" step 2; healing spec, "Staging snapshot").
//
// One page per call, and the page says which block it is as of. A node that
// has executed no block holds nothing anyone should start from and refuses
// (errors.NotReady) — as it will once the node states of step 5 land, for
// the whole time it is BOOTING.
//
// The request is validated before it is served. A cursor that names no
// stream, and a request that does not say which partition it means, are bad
// requests and not guesses: both are reachable over the wire, and the join of
// #4294 calls this in process, where a guess is fatal (#4291 review).
func (s *Sequencer) StagingSnapshot(_ context.Context, req *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	if err := req.Validate(); err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !strings.EqualFold(req.Partition, s.partitionID) {
		return nil, errors.BadRequest.WithFormat("requested partition is %s but this partition is %s", req.Partition, s.partitionID)
	}

	staging := s.stagingFor()
	if staging == nil {
		return nil, errors.NotReady.With("this node does not hold staging")
	}

	snap, size := staging.Snapshot(req)
	if snap.Block == 0 {
		return nil, errors.NotReady.With("this node has not executed a block")
	}

	// The page's size is measured as the page is built. Encoding it a second
	// time to weigh it would double the cost of the largest response this
	// node serves, and drop the error when that failed (#4291 review).
	mStagingSnapshots.WithLabelValues(s.partitionID).Inc()
	mStagingSnapshotBytes.WithLabelValues(s.partitionID).Add(float64(size))
	return snap, nil
}
