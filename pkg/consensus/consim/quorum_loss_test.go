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

// TestAPartitionThatLostItsQuorumSaysSo — #4512 (executor spec, "A partition
// that loses a quorum of its validators' states", 4). BVN1 loses two of its
// four validators for good. Its consensus cannot complete a round with two
// authors of the three a round needs, and it stops producing; what a stalled
// survivor reports must say which it is: two of four producing, a quorum of
// three needed -- a partition that lost its quorum, not a round that is
// merely slow (run 20261002T090546Z's BVN3 stall had four authors and two
// unvotable headers, and looked the same from outside).
func TestAPartitionThatLostItsQuorumSaysSo(t *testing.T) {
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
		TargetHeight:     400,
		Duration:         2 * time.Minute,
		StallAfter:       8 * time.Second,
		StateDir:         t.TempDir(),
		Restarts:         []Restart{{Part: "BVN1", Vals: []int{2, 3}, AtHeight: 40, Lose: true}},
		Out:              os.Stdout,
	})
	if err != nil {
		t.Fatalf("build sim: %v", err)
	}
	defer sim.Close()

	res, err := sim.Run(context.Background())
	if res != nil && res.Ok {
		t.Fatalf("BVN1 reached height 400 with two of four validators: the loss did not happen")
	}
	for _, v := range []int{0, 1} {
		rs := sim.byPart["BVN1"][v].n().Primary().RoundStatus()
		t.Logf("BVN1 node %d: round %d, %d of %d producing, quorum %d", v, rs.Round, rs.Authors, rs.Committee, rs.Quorum)
		if rs.Committee != 4 || rs.Quorum != 3 {
			t.Fatalf("node %d: committee %d quorum %d, want 4 and 3", v, rs.Committee, rs.Quorum)
		}
		if rs.Authors >= rs.Quorum {
			t.Fatalf("node %d reports %d of %d producing at its round: a partition that lost two validators does not say it lost its quorum", v, rs.Authors, rs.Committee)
		}
	}
	_ = err

	// Consim holds no anchors, so it cannot say N (the real source is tested
	// in internal/node/dagbft against real executors, and in test/e2e). What
	// it can pin is what N is read against: the survivors halt at one
	// executed height and have executed the same blocks, so each names the
	// same N and N is at or below that height (#4512).
	a, b := sim.byPart["BVN1"][0], sim.byPart["BVN1"][1]
	if a.height.Load() == 0 || a.height.Load() != b.height.Load() {
		t.Fatalf("the survivors halted at heights %d and %d, want one nonzero height", a.height.Load(), b.height.Load())
	}
	a.seqMu.Lock()
	b.seqMu.Lock()
	defer a.seqMu.Unlock()
	defer b.seqMu.Unlock()
	if len(a.seq) != len(b.seq) {
		t.Fatalf("the survivors executed %d and %d blocks", len(a.seq), len(b.seq))
	}
	for i := range a.seq {
		if a.seq[i] != b.seq[i] {
			t.Fatalf("the survivors executed different blocks at %d", i+1)
		}
	}
}
