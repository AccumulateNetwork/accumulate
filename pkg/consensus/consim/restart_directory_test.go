// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"context"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A Directory validator of twelve, stopped for a few seconds and restarted
// alone while the other eleven run on under load, catches up and executes
// what they executed. Run 20261003T191744Z (step C): bvn2-val4 was stopped
// for six seconds; its Directory restored at round 2098 and never collected
// past round 2104. For eleven minutes it asked for rounds 2106-2169 and
// received 1,100-1,800 certificates per ten seconds, every one already held
// in its pending buffer (verified=0, inserted=0), while the Directory moved
// on past round 3370.
func TestRestartOneDirectoryValidatorOfTwelve(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the full consensus stack for tens of seconds")
	}
	const restarted, restartAt, target = 7, 40, 400
	cfg := Config{
		BVNs:             3,
		ValidatorsPerBVN: 4,
		TPS:              20,
		MinRoundInterval: 5 * time.Millisecond,
		BatchTimeout:     10 * time.Millisecond,
		BatchSize:        20,
		TargetHeight:     target,
		TargetEveryNode:  true,
		Duration:         4 * time.Minute,
		StallAfter:       30 * time.Second,
		StateDir:         t.TempDir(),
		Restarts: []Restart{{
			Part: "Directory", Vals: []int{restarted}, AtHeight: restartAt,
			StopFirst: []int{restarted}, StopFirstFor: 2 * time.Second,
		}},
		Out: os.Stdout,
	}
	sim, err := New(cfg)
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if err != nil || !res.Ok {
		t.Fatalf("consensus did not resume after the restart: %v (reason %q, heights %v)", err, res.Reason, res.Heights)
	}
	if err := sim.CheckSequences("Directory"); err != nil {
		t.Fatalf("ordering differs across the restart: %v", err)
	}
	if h := sim.byPart["Directory"][restarted].height.Load(); h < target {
		// The run stops at Duration when the network is too slow to reach
		// target: a 2-CPU runner carries these 25 nodes at under a block a
		// second (#4558). Then the restarted validator must stand where its
		// peers stand, and they must have gone well past the restart; a
		// wedged one is left hundreds of rounds behind them.
		const moved, behind = 50, 10
		var top uint64
		for i, sn := range sim.byPart["Directory"] {
			if x := sn.height.Load(); i != restarted && x > top {
				top = x
			}
		}
		if top < restartAt+moved || h+behind < top {
			diagnoseWedge(t, sim, "Directory", restarted)
			t.Fatalf("the restarted Directory validator reached only height %d of %d; its peers reached %d", h, target, top)
		}
		t.Logf("the network reached only height %d of %d in %s; the restarted validator reached %d", top, target, cfg.Duration, h)
	}
	t.Logf("reached heights %v in %s", res.Heights, res.Elapsed.Truncate(time.Second))
}

// diagnoseWedge says why a node stopped collecting: its round and the
// highest round its DAG holds a quorum of, and what its pending buffer
// waits on.
func diagnoseWedge(t *testing.T, sim *Sim, part string, val int) {
	t.Helper()
	node := sim.byPart[part][val].n()
	quorum := node.Primary().Committee().QuorumCount()
	var top, frontier types.Round
	for _, r := range node.DAG().Rounds() {
		if r > top {
			top = r
		}
		if r > frontier && node.DAG().RoundCount(r) >= quorum {
			frontier = r
		}
	}
	held, onInserted, lacking := node.Primary().PendingDiagnosis()
	t.Logf("node %s/%d: round %d, DAG top %d, quorum frontier %d; pending %d, of which %d wait only on parents already in the DAG; %d parents lacked",
		part, val, node.Primary().CurrentRound(), top, frontier, held, onInserted, lacking)
}
