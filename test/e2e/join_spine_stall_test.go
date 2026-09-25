// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// A joining BVN node collects its partition's own anchors from its
// validators (executor spec, "Sync", "The algorithm", step 3; #4438), so a
// Directory pool with holes in it no longer holds it up. What holds it is its
// own partition's validators not signing: here one of BVN0's three nodes
// stops and another restarts, so the joining node's own sequencer is never
// asked (#4303), the stopped one answers nothing, and the one that answers
// signs alone where the partition's threshold is two. The anchor is produced and no quorum
// signed it, and the production join says so on the gauge -- in the join's
// round and in the root watch's read (#4419, review F2).
func TestAJoinHeldAtAnAnchorItsValidatorsDoNotSignSaysSo(t *testing.T) {
	sim, _, _, _ := joinADirectoryNodeByPull(t)
	ctx := context.Background()
	b := sim.S.Partition("BVN0")
	const bvnJoiner, other = 1, 2

	b.StopNode(other)
	b.RestartNode(bvnJoiner)
	require.Equal(t, float64(-1), spineStalledGauge(t, "BVN0"), "a join built fresh is not stalled")
	_ = b.NodeJoinState(bvnJoiner).Pull(ctx)
	require.Greater(t, spineStalledGauge(t, "BVN0"), float64(0), "the join's round does not report the stall")

	// Diverged, the root watch's read after the handoff.
	b.RestartNode(bvnJoiner)
	require.Equal(t, float64(-1), spineStalledGauge(t, "BVN0"), "a join built fresh is not stalled")
	_, _, _ = b.NodeJoinState(bvnJoiner).Diverged(ctx)
	require.Greater(t, spineStalledGauge(t, "BVN0"), float64(0), "the root watch's read does not report the stall")
}

// spineStalledGauge is accumulate_join_spine_stalled_entry for a partition,
// as the node's metrics endpoint serves it.
func spineStalledGauge(t *testing.T, partition string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != "accumulate_join_spine_stalled_entry" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "partition" && l.GetValue() == partition {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	t.Fatalf("no accumulate_join_spine_stalled_entry series for %s", partition)
	return 0
}
