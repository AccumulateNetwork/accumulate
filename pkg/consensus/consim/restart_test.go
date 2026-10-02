// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"context"
	"log/slog"
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
	runRestartOf(t, Restart{Part: "BVN1", Vals: vals, AtHeight: 40, ExecutorFirst: executorFirst}, nil)
}

// runRestartOf is runRestart with the restart given whole, and the
// configuration adjusted by tweak.
func runRestartOf(t *testing.T, r Restart, tweak func(*Config)) {
	t.Helper()
	vals := r.Vals
	if testing.Short() {
		t.Skip("runs the full consensus stack for tens of seconds")
	}
	if os.Getenv("CONSIM_DEBUG") != "" {
		// The vote gates say why a header is not voted only at Debug.
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
	cfg := Config{
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
		Restarts:         []Restart{r},
		Out:              os.Stdout,
	}
	if tweak != nil {
		tweak(&cfg)
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

// TestRestartAfterTwoNodesStopFirst stops two of BVN1's four validators,
// lets the other two run on for a moment, then stops them too and restarts
// all four from their checkpoints. The survivors author the round after
// the last certified one, with batches, and no vote can certify it; the
// blocks already ordered are still executed and checkpointed, and before
// the fix each checkpoint's Retain removed every batch no certificate
// named -- the restored header came back without its payload, no peer
// could vote for it, its author could not author the round again, and the
// partition held at two certificates of the three it needed (run
// 20261002T090546Z, BVN3 at round 25839, block 12920).
func TestRestartAfterTwoNodesStopFirst(t *testing.T) {
	runRestartOf(t,
		Restart{Part: "BVN1", Vals: []int{0, 1, 2, 3}, AtHeight: 40, StopFirst: []int{2, 3}, StopFirstFor: 500 * time.Millisecond},
		func(c *Config) {
			// Rounds slow enough, under load enough, that a header carries
			// a batch: at 5 ms rounds and 20 tx/s almost every header is
			// empty, and an empty header needs no batch to be voted.
			c.TPS = 200
			c.MinRoundInterval = 20 * time.Millisecond
		})
}

// runNetworkRestart restarts every validator of every partition together, as
// the Docker network was recreated on 2026-09-28, with the executor keeping
// no empty block, and with the load to the partitions in loadUntil stopped
// once they reach that height, so their last blocks before the restart
// carry nothing. Each such executor's last block is then below both of its
// checkpoints; on Docker the BVN validators started at round zero (#4448).
// Every node of every partition must resume, order what its peers order and
// go on producing: sustained blocks past its height at the restart.
func runNetworkRestart(t *testing.T, loadUntil map[string]uint64) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the full consensus stack for tens of seconds")
	}
	const restartAt, sustained = 60, 150
	sim, err := New(Config{
		BVNs:               1,
		ValidatorsPerBVN:   4,
		TPS:                20,
		MinRoundInterval:   5 * time.Millisecond,
		BatchTimeout:       10 * time.Millisecond,
		BatchSize:          20,
		TargetHeight:       restartAt + 30 + sustained,
		Duration:           3 * time.Minute,
		StallAfter:         20 * time.Second,
		StateDir:           t.TempDir(),
		DiscardEmptyBlocks: true,
		LoadUntilHeight:    loadUntil,
		Restarts:           []Restart{{Part: "BVN1", Network: true, AtHeight: restartAt}},
		Out:                os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if err != nil || !res.Ok {
		t.Fatalf("the network did not resume after the restart: %v (reason %q, heights %v)", err, res.Reason, res.Heights)
	}
	for _, part := range []string{"Directory", "BVN1"} {
		if err := sim.CheckSequences(part); err != nil {
			t.Fatalf("ordering differs across the restart: %v", err)
		}
		if n := sim.Equivocations(part); n != 0 {
			t.Fatalf("%s: %d conflicting certificates refused", part, n)
		}
		for i, sn := range sim.byPart[part] {
			if sn.resumedAt == 0 {
				t.Fatalf("%s node %d was not restarted", part, i)
			}
			if h := sn.height.Load(); h < sn.resumedAt+sustained {
				t.Fatalf("%s node %d resumed at block %d and produced only to %d; want %d blocks after it", part, i, sn.resumedAt, h, sustained)
			}
			if _, quiet := loadUntil[part]; quiet && sn.keptAtRestart >= sn.resumedAt {
				t.Fatalf("%s node %d kept block %d at the restart: its last blocks were not empty, so the test did not restart the shape it names", part, i, sn.keptAtRestart)
			}
		}
	}
	t.Logf("reached heights %v in %s", res.Heights, res.Elapsed.Truncate(time.Second))
}

// TestRestartNetworkAfterEmptyBlocks is the Docker shape: the Directory busy
// up to the restart, BVN1's last blocks empty.
func TestRestartNetworkAfterEmptyBlocks(t *testing.T) {
	runNetworkRestart(t, map[string]uint64{"BVN1": 20})
}

// TestRestartNetworkAllIdle restarts the network with every partition's last
// blocks empty, the Directory's included: the shape the Docker network had
// by 23:44Z, the Directory producing only empty blocks (1739 of 1745) with
// its executor at 12410 and its checkpoints at 14142 and 14143.
func TestRestartNetworkAllIdle(t *testing.T) {
	runNetworkRestart(t, map[string]uint64{"BVN1": 20, "Directory": 20})
}
