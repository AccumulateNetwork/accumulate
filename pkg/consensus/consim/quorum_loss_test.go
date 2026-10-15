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

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/primary"
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
	// A member counts as producing for primary.HeardWithin after the last
	// header or vote it sent, and the run stops StallAfter after BVN1's last
	// block, which is sooner: read then, a lost validator's last header still
	// counts. Read once HeardWithin has passed: the lost two have aged out,
	// and the two survivors, still running, are still heard (#4558).
	time.Sleep(primary.HeardWithin)
	for _, v := range []int{0, 1} {
		rs := sim.byPart["BVN1"][v].n().Primary().RoundStatus()
		t.Logf("BVN1 node %d: round %d, %d of %d producing, quorum %d (%+v)", v, rs.Round, rs.Authors, rs.Committee, rs.Quorum, rs)
		if rs.Committee != 4 || rs.Quorum != 3 {
			t.Fatalf("node %d: committee %d quorum %d, want 4 and 3", v, rs.Committee, rs.Quorum)
		}
		if rs.Authors != 2 {
			t.Fatalf("node %d reports %d of %d producing at its round, want 2: two of four survive, a quorum of three is needed", v, rs.Authors, rs.Committee)
		}
	}
	_ = err
}
