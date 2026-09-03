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

// A network must not REFUSE work it is healthy enough to take.
//
// This is the failure the simulator could not previously see, because load()
// discarded the submit error. Refusal is the one stage whose failure leaves
// every downstream gauge looking healthy: the transaction never enters the
// pipeline, so rounds advance, headers are created, votes flow, certificates
// form and commit — and the work is simply gone.
//
// Observed in soak 20260903T035139Z: BVN2 produced 46,428 messages for BVN1,
// BVN1 received 10,378, and both watermarks sat frozen while BVN1 produced
// blocks at ~6/s for the entire run. What was being refused included the
// cross-partition synthetics themselves and the healer's own re-submissions,
// so the stream that needed the queue to drain was the stream being turned
// away. Twenty minutes per attempt in Docker; seconds here.
//
// The store is deliberately tiny so it fills almost immediately — the real
// default is 1000 batches, which takes a soak's throughput and minutes to
// reach, and the question is what happens AFTER it fills, not how long it
// takes to get there.
func TestFullBatchStoreMustNotRefuseWork(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the consensus stack for ~20s")
	}

	sim, err := New(Config{
		BVNs:             1,
		ValidatorsPerBVN: 4,
		TPS:              200,
		MinRoundInterval: 5 * time.Millisecond,
		BatchTimeout:     10 * time.Millisecond,
		BatchSize:        4,

		// Small enough to fill in the first seconds of load, but not smaller
		// than the network's working set: a header names several batches and
		// pins them all while its vote is deferred, so a store below that is
		// degenerate — eviction can free nothing and the run wedges on the
		// fixture rather than on the behaviour under test.
		MaxStoredBatches: 120,
		MaxPendingCount:  8,
		MaxPendingSize:   8192,

		TargetHeight: 40,
		Duration:     45 * time.Second,
		StallAfter:   15 * time.Second,
		Out:          os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if res == nil {
		t.Fatalf("run: %v", err)
	}
	t.Logf("submitted=%d refused=%d heights=%v elapsed=%s",
		res.Submitted, res.Refused, res.Heights, res.Elapsed.Round(time.Millisecond))

	// THE ASSERTION. A full store is a reason to seal, not a reason to refuse:
	// accepting a transaction does not grow the store, only sealing does, and
	// crossing a pending boundary is the signal to cut a batch.
	if res.Refused != 0 {
		t.Errorf("network refused %d of %d submissions with a full batch store; "+
			"a refused transaction never enters the pipeline, and what gets refused "+
			"under load includes cross-partition messages and heal re-submissions",
			res.Refused, res.Submitted)
	}

	// And it must still make progress while the store is full — refusing
	// nothing is worthless if the network merely stops instead.
	if err != nil {
		t.Errorf("network did not progress with a full batch store: %v (%s)", err, res.Reason)
	}
}
