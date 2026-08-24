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
