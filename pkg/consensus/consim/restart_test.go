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
)

// runRestart runs one BVN of four validators (every validator hosting the
// Directory too) under load, restarts the named BVN1 nodes at height 40 from
// their persisted checkpoints, and runs on to height 160. With
// executorFirst, the nodes' executors stop that long before the nodes, so
// consensus runs on past their last checkpoint. It fails on a
// stall, on nodes that executed different blocks, and on any equivocation.
func runRestart(t *testing.T, executorFirst time.Duration, vals ...int) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the full consensus stack for tens of seconds")
	}
	sim, err := New(Config{
		BVNs:             1,
		ValidatorsPerBVN: 4,
		TPS:              20,
		MinRoundInterval: 5 * time.Millisecond,
		BatchTimeout:     10 * time.Millisecond,
		BatchSize:        20,
		TargetHeight:     160,
		Duration:         3 * time.Minute,
		StallAfter:       20 * time.Second,
		StateDir:         t.TempDir(),
		Restarts:         []Restart{{Part: "BVN1", Vals: vals, AtHeight: 40, ExecutorFirst: executorFirst}},
		Out:              os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if err != nil || !res.Ok {
		t.Fatalf("consensus did not resume after the restart: %v (reason %q, heights %v)", err, res.Reason, res.Heights)
	}
	for _, part := range []string{"Directory", "BVN1"} {
		if err := sim.CheckSequences(part); err != nil {
			t.Fatalf("ordering differs across the restart: %v", err)
		}
		if n := sim.Equivocations(part); n != 0 {
			t.Fatalf("%s: %d conflicting certificates refused: a restarted node authored a round it had authored before", part, n)
		}
	}
	for _, v := range vals {
		if h := sim.byPart["BVN1"][v].height.Load(); h < 160 {
			t.Fatalf("restarted BVN1 node %d reached only height %d", v, h)
		}
	}
	t.Logf("reached heights %v in %s", res.Heights, res.Elapsed.Truncate(time.Second))
}

// TestRestartWholePartition stops every validator of BVN1 mid-run and
// restarts them all from their persisted state (#4448). Before the
// checkpoint carried the DAG's tail, no node held a parent certificate for
// its restored round and none could author a header again: the partition
// stalled for ever, as the 12-validator Docker network did at block 40060.
func TestRestartWholePartition(t *testing.T) {
	runRestart(t, 0, 0, 1, 2, 3)
}

// TestRestartWholePartitionExecutorBehind is the whole-partition restart
// with every executor stopped before its node, so each node's last
// checkpoint is rounds behind the certificates it held and the headers it
// authored: those rounds are on disk only because authoring wrote them.
func TestRestartWholePartitionExecutorBehind(t *testing.T) {
	runRestart(t, 300*time.Millisecond, 0, 1, 2, 3)
}

// TestRestartOneNode restarts one BVN1 validator mid-run while its peers go
// on: it catches up and executes what they executed.
func TestRestartOneNode(t *testing.T) {
	runRestart(t, 0, 2)
}

// TestRestartDoesNotReauthorARound restarts every BVN1 validator but one,
// with their executors stopped first, so each restarted node's checkpoint
// is rounds behind the headers it authored and the survivor holds their
// certificates. A restarted node must not author any of those rounds again:
// a second certificate for one is refused by the survivor as equivocation
// (#4159 stall 3, #4448).
func TestRestartDoesNotReauthorARound(t *testing.T) {
	runRestart(t, 300*time.Millisecond, 0, 1, 3)
}
