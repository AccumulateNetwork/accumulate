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

// TestSoakTopologyLiveness runs the full consensus stack in the soak's dual
// topology (every validator hosts the Directory node and its BVN node on one
// shared libp2p host) at millisecond pacing, under sustained load, to a
// height well past where the Docker soak deterministically froze (DN 529-553,
// #4159). Before the batch-recovery fixes (61af72d1d) this stalled in 5 of 6
// runs; with them it passes in ~45-60s. A stall fails the test and prints the
// per-node stage-freeze diagnosis, naming the pipeline stage that stopped.
//
// Height counts BLOCKS, and a block is one committed leader group (#4164) —
// roughly one per two rounds — not one per certificate as before, so the
// target is ~12x lower than the old per-certificate 2000 for the same length
// of consensus history.
func TestSoakTopologyLiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the full 24-node consensus stack for ~a minute")
	}

	sim, err := New(Config{
		BVNs:             3,
		ValidatorsPerBVN: 4,
		TPS:              20,
		MinRoundInterval: 5 * time.Millisecond,
		BatchTimeout:     10 * time.Millisecond,
		BatchSize:        20,
		TargetHeight:     300,
		Duration:         3 * time.Minute,
		StallAfter:       20 * time.Second,
		Out:              os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if err != nil {
		t.Fatalf("consensus liveness failed: %v (reason %q, heights %v)", err, res.Reason, res.Heights)
	}
	if !res.Ok {
		t.Fatalf("run did not reach the target: reason %q, heights %v", res.Reason, res.Heights)
	}
	t.Logf("reached heights %v in %s", res.Heights, res.Elapsed.Truncate(time.Second))
}

// TestSkewedLoad_HeavyPartitionLagsButDoesNotWedge asks the question soak
// 20260831T070855Z left open.
//
// That run died at 0.28h with BVN2 frozen at height 277 — 23.1 s/block and
// then nothing — while BVN1 held a flat 3.0 s/block to 368 and the Directory
// followed BVN2 down. BVN2 was not lock-blocked (no semacquire anywhere in its
// goroutine dumps) and was not idle (37-57% CPU against BVN1's 14-19%). What
// it was, was loaded: twice the database, 3.3x the synthetics to the other
// BVN, 15x to the Directory.
//
// So: does a partition carrying several times its peers' load STOP, or does it
// only fall behind? This runs the soak's own topology (2 BVNs x 4 validators,
// every validator hosting the Directory too) with BVN2 at 4x BVN1.
//
// consim models CONSENSUS, and its executor only counts — there is no
// synthetic delivery, no anchoring, no storage. That is what makes this a
// bisect rather than a mere reproduction. A wedge here puts the cause in the
// consensus pipeline, and the stage-freeze diagnosis names it. No wedge here
// puts the cause in the layers consim omits, and the Docker evidence should be
// read against those instead.
func TestSkewedLoad_HeavyPartitionLagsButDoesNotWedge(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the full consensus stack for ~a minute")
	}

	sim, err := New(Config{
		BVNs:             2,
		ValidatorsPerBVN: 4,
		TPS:              20,
		TPSByPartition:   map[string]int{"BVN2": 80},
		MinRoundInterval: 5 * time.Millisecond,
		BatchTimeout:     10 * time.Millisecond,
		BatchSize:        20,
		TargetHeight:     300,
		Duration:         3 * time.Minute,
		StallAfter:       20 * time.Second,
		Out:              os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if err != nil {
		t.Fatalf("a partition under 4x load stalled consensus: %v (reason %q, heights %v)",
			err, res.Reason, res.Heights)
	}
	if !res.Ok {
		t.Fatalf("run did not reach the target: reason %q, heights %v", res.Reason, res.Heights)
	}
	t.Logf("heights %v in %s — the heavy partition kept producing",
		res.Heights, res.Elapsed.Truncate(time.Second))
}
