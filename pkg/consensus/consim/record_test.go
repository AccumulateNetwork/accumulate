// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"strings"
	"testing"
)

// A node that resumes below the blocks it executed executes them again, and
// they must be the blocks it executed before: the record drops them at the
// resume, then checks each one executed again against what it dropped
// (#4558).
func TestABlockExecutedAgainAfterARestartMustBeTheSame(t *testing.T) {
	sim := &Sim{byPart: map[string][]*simNode{"BVN1": {{part: "BVN1"}, {part: "BVN1"}}}}
	a, b := sim.byPart["BVN1"][0], sim.byPart["BVN1"][1]
	for _, e := range []string{"2:x", "4:y", "6:z"} {
		a.record(e)
		b.record(e)
	}

	a.resumeRecord(1)
	a.record("4:y")
	if err := sim.CheckSequences("BVN1"); err != nil {
		t.Fatalf("the same block executed again is reported: %v", err)
	}

	a.record("8:w")
	err := sim.CheckSequences("BVN1")
	if err == nil || !strings.Contains(err.Error(), "block 3") {
		t.Fatalf("block 3 executed as 8:w after the restart and 6:z before: got %v", err)
	}
}
