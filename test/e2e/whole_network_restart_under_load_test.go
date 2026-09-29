// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAWholeNetworkRestartUnderLoadKeepsEveryStreamDelivering — #4465. The
// Docker run 20260929T042606Z restarted every validator at once under
// cross-partition load, and from then on BVN1 held every synthetic from BVN2
// between Delivered+1 and Received and executed none until it ran out of
// memory. Here the same: two BVNs pay each other continuously, BVN1 hears
// nothing from BVN0 for a stretch so synthetics, their proofs and anchors are
// in flight, and every validator of every partition restarts at once through
// the production join. Afterwards, under the same load, every stream keeps
// delivering: each synthetic stream's Delivered advances in every window and
// reaches what its source produced.
func TestAWholeNetworkRestartUnderLoadKeepsEveryStreamDelivering(t *testing.T) {
	wholeNetworkRestartUnderLoad(t, 3, func(messaging.Message) bool { return false })
}

// Top-level rather than subtests: the join's peers are found by the network's
// name, which is the test's, and a subtest's name carries a slash.

// BVN1 hears neither BVN0's synthetics nor the Directory's anchors before the
// restart.
func TestAWholeNetworkRestartUnderLoadWithBothHeld(t *testing.T) {
	wholeNetworkRestartUnderLoad(t, 3, heldFromBVN1)
}

// BVN1 hears BVN0's synthetics but not the Directory's anchors, so what it
// holds at the restart is collected ahead of its anchor.
func TestAWholeNetworkRestartUnderLoadWithAnchorsHeld(t *testing.T) {
	wholeNetworkRestartUnderLoad(t, 3, heldAnchorFromBVN1)
}

// As above, with four validators per partition, as the Docker network has.
func TestAWholeNetworkRestartUnderLoadWithAnchorsHeldFourValidators(t *testing.T) {
	wholeNetworkRestartUnderLoad(t, 4, heldAnchorFromBVN1)
}

// BVN1 hears the Directory's anchors but not BVN0's synthetics.
func TestAWholeNetworkRestartUnderLoadWithSyntheticsHeld(t *testing.T) {
	wholeNetworkRestartUnderLoad(t, 3, func(m messaging.Message) bool { return !isAnchor(m) && heldFromBVN1(m) })
}

func heldAnchorFromBVN1(m messaging.Message) bool { return isAnchor(m) && heldFromBVN1(m) }

func isAnchor(m messaging.Message) bool { _, ok := m.(*messaging.BlockAnchor); return ok }

func wholeNetworkRestartUnderLoad(t *testing.T, nodes int, held func(messaging.Message) bool) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	var hold atomic.Bool
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, nodes),
		simulator.CaptureDispatchedMessages(func(_ context.Context, env *messaging.Envelope) (bool, error) {
			if !hold.Load() {
				return true, nil
			}
			for _, m := range env.Messages {
				if held(m) {
					return false, nil
				}
			}
			return true, nil
		}),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	for _, a := range []struct {
		id  *url.URL
		key []byte
	}{{alice, aliceKey}, {bob, bobKey}} {
		MakeIdentity(t, sim.DatabaseFor(a.id), a.id, a.key[32:])
		CreditCredits(t, sim.DatabaseFor(a.id), a.id.JoinPath("book", "1"), 1e12)
		MakeAccount(t, sim.DatabaseFor(a.id), &TokenAccount{Url: a.id.JoinPath("tokens"), TokenUrl: AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(a.id), a.id.JoinPath("tokens"), big.NewInt(1e9))
	}

	// Load: alice (BVN0) pays bob (BVN1) and bob pays alice, without waiting
	var ts uint64
	load := func() {
		t.Helper()
		for i := 0; i < 2; i++ {
			ts++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(bob, "tokens").
					SendTokens(1, 0).To(alice, "tokens").
					SignWith(bob, "book", "1").Version(1).Timestamp(ts).PrivateKey(bobKey))
		}
	}

	type pair struct{ src, dst string }
	streams := []pair{{"BVN0", "BVN1"}, {"BVN1", "BVN0"}}
	delivered := func(p pair, node int) uint64 {
		return syntheticLedger(t, sim.S.Partition(p.dst).NodeDatabase(node), p.dst).Partition(PartitionUrl(p.src)).Delivered
	}
	produced := func(p pair) uint64 {
		return syntheticLedger(t, sim.S.Partition(p.src).NodeDatabase(0), p.src).Partition(PartitionUrl(p.dst)).Produced
	}
	report := func(when string) {
		t.Helper()
		for _, p := range streams {
			var ds []uint64
			for i := 0; i < nodes; i++ {
				ds = append(ds, delivered(p, i))
			}
			t.Logf("%s: %s->%s produced %d delivered %v", when, p.src, p.dst, produced(p), ds)
		}
		for _, bvn := range []string{"BVN0", "BVN1"} {
			t.Logf("%s: DN->%s anchors delivered %d", when, bvn,
				anchorLedger(t, sim.S.Partition(bvn).NodeDatabase(0), bvn).Partition(DnUrl()).Delivered)
		}
	}

	// Steady load before the restart
	for i := 0; i < 10; i++ {
		load()
		sim.StepN(5)
	}

	// BVN1 hears nothing from BVN0 or the Directory for a stretch, still
	// under load, so synthetics, proofs and anchors are in flight when every
	// validator restarts at once
	hold.Store(true)
	for i := 0; i < 5; i++ {
		load()
		sim.StepN(5)
	}
	load()
	report("before the restart")

	var all []restartTarget
	for _, name := range []string{Directory, "BVN0", "BVN1"} {
		p := sim.S.Partition(name)
		for i := 0; i < p.NodeCount(); i++ {
			all = append(all, restartTarget{name, p, i})
		}
	}
	restartAndJoin(t, sim, all)
	hold.Store(false)
	report("after the join")

	// Under load, every window: every node's Delivered on every synthetic
	// stream advances
	last := map[pair][]uint64{}
	for _, p := range streams {
		for i := 0; i < nodes; i++ {
			last[p] = append(last[p], delivered(p, i))
		}
	}
	const windows = 15
	for w := 0; w < windows; w++ {
		for i := 0; i < 4; i++ {
			load()
			sim.StepN(5)
		}
		for _, p := range streams {
			for i := 0; i < nodes; i++ {
				d := delivered(p, i)
				if d <= last[p][i] {
					report("stalled")
					logHeadBlocked(t)
				}
				require.Greater(t, d, last[p][i], "window %d: %s node %d delivered nothing from %s (Delivered %d, %s produced %d)",
					w, p.dst, i, p.src, d, p.src, produced(p))
				last[p][i] = d
			}
		}
	}

	// Load stops; everything produced is delivered
	sim.StepN(100)
	report("drained")
	for _, p := range streams {
		for i := 0; i < nodes; i++ {
			require.Equal(t, produced(p), delivered(p, i), "%s node %d did not deliver everything %s produced for it", p.dst, i, p.src)
		}
	}
}

// logHeadBlocked reports what accumulate_exec_head_blocked_total says of
// blocked stream heads, by stream kind and reason.
func logHeadBlocked(t *testing.T) {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != "accumulate_exec_head_blocked_total" {
			continue
		}
		for _, m := range mf.Metric {
			var labels []string
			for _, l := range m.Label {
				labels = append(labels, l.GetName()+"="+l.GetValue())
			}
			t.Logf("head blocked {%s} = %v", strings.Join(labels, ","), m.Counter.GetValue())
		}
	}
}
