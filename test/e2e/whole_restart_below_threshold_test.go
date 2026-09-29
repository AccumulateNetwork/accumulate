// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestAWholeRestartWhoseFirstResumersAreBelowTheThresholdResumesTheRest —
// #4466. Every validator of a partition restarts at once, and one of them
// takes the whole-restart exit before the others ask: it resumes from its own
// block and serves again. The others then find a peer serving, and before
// #4466 that sent them down the ordinary join, which pulls a peer's state and
// waits for a signed anchor to match it against. Only the validators that are
// executing sign an anchor, and one is below the partition's threshold, so no
// anchor after the restart ever reaches its quorum: the joiners waited for
// ever, and every stream carrying the partition's anchors held behind the one
// below its quorum ("only 1 of Directory's validators signed, and 4 … are
// required"; in Docker, "only 6 of Directory's validators signed, and 8").
//
// The signatures were never lost. They were never made: a joining validator
// executes nothing, so it signs nothing. A validator whose store is its own
// execution resumes while fewer of its peers serve than the threshold that
// signs an anchor, because until one more of them executes, no anchor it could
// match is ever signed.
//
// The Directory is the partition: six validators, a threshold of four. The
// joins run through the production wiring, as restartAndJoin runs them.
func TestAWholeRestartWhoseFirstResumersAreBelowTheThresholdResumesTheRest(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.StepN(20)

	dn := sim.S.Partition(Directory)
	var all []restartTarget
	for i := 0; i < dn.NodeCount(); i++ {
		all = append(all, restartTarget{Directory, dn, i})
	}
	require.Len(t, all, 6)

	delivered := func() uint64 {
		return anchorLedger(t, sim.S.Partition("BVN0").NodeDatabase(0), "BVN0").Partition(DnUrl()).Delivered
	}

	for _, n := range all {
		n.part.RestartNode(n.node)
		require.True(t, n.part.Joining(n.node))
	}

	// The first: every peer it asks is joining, so it resumes.
	exits := joinRestarted(t, sim, all[:1], 60*time.Second)
	require.True(t, exits[0], "the first Directory validator did not resume from its own block")

	// The rest find one peer serving, and one signature is not four.
	before := delivered()
	exits = joinRestarted(t, sim, all[1:], 30*time.Second)
	resumed := 1
	for _, e := range exits {
		if e {
			resumed++
		}
	}
	require.GreaterOrEqual(t, resumed, 4, "fewer Directory validators resumed than sign an anchor")

	// The Directory's anchors reach their quorum again, and a BVN executes them.
	sim.StepN(50)
	require.Greater(t, delivered(), before, "no Directory anchor reached its quorum after the restart")
}
