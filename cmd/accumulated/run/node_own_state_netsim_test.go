// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A NODE MUST BE ABLE TO REPORT ITS OWN STATE AND ITS OWN HEIGHT. A whole day
// of monitoring a twelve-node network said "healthy" over a member that had
// executed nothing for twenty minutes, because no reading available to the
// harness could say otherwise (#4345).
//
// This runs a real netsim — the daemon's own start path, its own executor,
// its own metrics registry — because the defects were in the wiring and not
// in any library. Nothing here is built by hand: the assertions read the
// process's own Prometheus registry, which is what the /metrics endpoint
// serves, and the node under test runs the Directory AND a BVN in one
// process, which is the shape of the live network and the shape the
// unlabelled counters could not describe.
//
// Measured live on 2026-09-18, and reproduced here before the fix:
//
//	(a) accumulate_node_state was created by the join and only by the join,
//	    so a node that never joined exported NO SUCH SERIES. This netsim node
//	    never joins (it is the first node of a fresh network), so the series
//	    is absent here exactly as it was absent on acc-bvn1-val2.
//	(b) Nothing said what block THIS node had executed, and the two block
//	    counters had no partition label: 5264 on acc-bvn1-val2, one series
//	    summing two chains.
func TestANodeReportsItsOwnStateAndItsOwnHeight(t *testing.T) {
	api := startNetsimAndExecute(t)

	// (a) A node that never joined exports its state, for every partition it
	// runs. 2 is ACTIVE: it executes without asking anyone and answers for
	// what it holds. Before the fix there is no series to read.
	t.Run("StateIsExportedByANodeThatNeverJoined", func(t *testing.T) {
		g := gaugeByPartition(t, "accumulate_node_state")
		require.Contains(t, g, "directory",
			"a node that never joined exported no state for the Directory: absence cannot assert health (#4345a)")
		require.Contains(t, g, "bvn1",
			"a node that never joined exported no state for its BVN: absence cannot assert health (#4345a)")
		require.Equal(t, float64(2), g["directory"])
		require.Equal(t, float64(2), g["bvn1"])
	})

	// (b) The node exports its own executed block, per partition, and it is
	// the height its own executor reached — checked against the ledger this
	// node executed into, read through its own API.
	t.Run("ExecutedHeightIsExportedPerPartition", func(t *testing.T) {
		g := gaugeByPartition(t, "accumulate_node_executed_block")
		require.Contains(t, g, "directory", "nothing said what block this node executed (#4345b)")
		require.Contains(t, g, "bvn1", "nothing said what block this node executed (#4345b)")
		require.Greater(t, g["directory"], float64(protocol.GenesisBlock))
		require.Greater(t, g["bvn1"], float64(protocol.GenesisBlock))

		// Agreement with the height the node executed into. The two are read
		// a moment apart on a running network and the API's view lags its
		// own commit by a block, so a small skew either way is expected — a
		// gauge that is not this node's own height is out by far more than
		// three at block eleven.
		for part, key := range map[string]string{"dn": "directory", "bvn-BVN1": "bvn1"} {
			idx, err := ledgerIndex(api, part)
			require.NoError(t, err)
			require.LessOrEqual(t, math.Abs(float64(idx)-g[key]), float64(3),
				"the %s gauge (%v) is not this node's own executed block (%v)", key, g[key], idx)
		}
	})

	// (b, second half) The block counters name their partition, so one series
	// is one chain rather than the sum of two.
	t.Run("TheBlockCountersNameTheirPartition", func(t *testing.T) {
		for _, name := range []string{
			"accumulate_exec_blocks_total",
			"accumulate_dagbft_blocks_produced_total",
		} {
			c := counterByPartition(t, name)
			require.Contains(t, c, "Directory", "%s does not name its partition (#4345b)", name)
			require.Contains(t, c, "BVN1", "%s does not name its partition (#4345b)", name)

			// Each series is strictly less than the process total: that is
			// what "one series summing two chains" meant, and what the label
			// undoes.
			total := c["Directory"] + c["BVN1"]
			require.Less(t, c["Directory"], total, "%s: the Directory's count is the whole process's", name)
			require.Less(t, c["BVN1"], total, "%s: the BVN's count is the whole process's", name)
		}
	})
}

// startNetsimAndExecute starts a one-BVN, one-validator network in this
// process, waits until it has executed blocks of its own, and returns its API
// address.
//
// It waits on the process's total block count, which exists with or without
// the partition label, so the wait is not itself a test of the fix.
func startNetsimAndExecute(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rootDir := t.TempDir()
	basePort := freeDevnetBase(t, 1)

	// The Prometheus registry is the PROCESS's, and other tests in this
	// package start networks of their own into it. What this network produced
	// is therefore a delta, not a total — waiting on the total returns at
	// once on an earlier test's counters and leaves this node's gauges
	// unwritten.
	before := familyTotal(t, "accumulate_dagbft_blocks_produced_total")

	cfg := &Config{
		Network: "OwnState",
		Logging: &Logging{
			Format: "plain",
			Rules:  []*LoggingRule{{Level: slog.LevelError}},
		},
		P2P: &P2P{Key: &PrivateKeySeed{Seed: record.NewKey("node-own-state")}},
		Configurations: []Configuration{
			&NetSimConfiguration{
				Listen:     multiaddr.StringCast(fmt.Sprintf("/tcp/%d", basePort)),
				Bvns:       1,
				Validators: 1,
				Globals: &network.GlobalValues{
					Globals: &protocol.NetworkGlobals{
						// Never during the test: a major block adds nothing
						// here and costs time.
						MajorBlockSchedule: "0 0 1 1 *",
					},
				},
			},
		},
	}
	cfg.file = filepath.Join(rootDir, "accumulate.toml")

	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = rootDir
	require.NoError(t, inst.Start())
	t.Cleanup(inst.Stop)

	// Both partitions must have executed several blocks of their own. Twenty
	// across the two is ten each at a one-second cadence — enough that a
	// height which is not this node's is unmistakably out.
	var produced float64
	for i := 0; i < 120 && produced < 20; i++ {
		time.Sleep(time.Second)
		produced = familyTotal(t, "accumulate_dagbft_blocks_produced_total") - before
	}
	require.GreaterOrEqual(t, produced, float64(20), "the netsim produced almost no blocks")

	// And the API must answer, because the height the gauge is checked
	// against is read through it.
	api := fmt.Sprintf("http://127.0.0.1:%d/v3", basePort+int(portAccAPI))
	var ready bool
	for i := 0; i < 60 && !ready; i++ {
		_, dn := ledgerIndex(api, "dn")
		_, bvn := ledgerIndex(api, "bvn-BVN1")
		ready = dn == nil && bvn == nil
		if !ready {
			time.Sleep(time.Second)
		}
	}
	require.True(t, ready, "the API never answered for both partitions")
	return api
}

// --- the process's own Prometheus registry ---------------------------------

func gatherMetric(t *testing.T, name string) *dto.MetricFamily {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

func gaugeByPartition(t *testing.T, name string) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	mf := gatherMetric(t, name)
	if mf == nil {
		return out
	}
	for _, m := range mf.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "partition" {
				out[l.GetValue()] = m.GetGauge().GetValue()
			}
		}
	}
	return out
}

func counterByPartition(t *testing.T, name string) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	mf := gatherMetric(t, name)
	if mf == nil {
		return out
	}
	for _, m := range mf.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "partition" {
				out[l.GetValue()] = m.GetCounter().GetValue()
			}
		}
	}
	return out
}

// familyTotal sums every series of a counter family, labelled or not.
func familyTotal(t *testing.T, name string) float64 {
	t.Helper()
	mf := gatherMetric(t, name)
	if mf == nil {
		return 0
	}
	var total float64
	for _, m := range mf.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	return total
}
