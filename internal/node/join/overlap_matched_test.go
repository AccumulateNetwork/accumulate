// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The blocks matched count only what was verified, in an unbroken run since
// the state was proven: a block settled against its record, or a block under
// an anchored root the node's own root matched. A frontier block -- settled
// with no record to compare -- stops the run until an anchor covers it; a
// mismatch or a demotion sets it to 0 (Paul, 2026-10-09).
func TestTheBlocksMatchedCountOnlyWhatWasVerified(t *testing.T) {
	matched := func() float64 { return testutil.ToFloat64(mMatchedBlocks.WithLabelValues("matched-test")) }
	o := &overlap{partition: protocol.PartitionUrl("BVN1"), label: "matched-test", log: slog.Default(), blocks: 20}

	o.start(100)
	o.done(101, true, 0)
	o.done(102, true, 0)
	require.Equal(t, 2.0, matched(), "two blocks settled against their records")

	o.done(103, false, 0) // a frontier block: no record to compare
	o.done(104, true, 0)
	require.Equal(t, 2.0, matched(), "a block never compared was counted as matched")

	o.vouched(104) // an anchored root after 104 matched this node's
	require.Equal(t, 4.0, matched(), "an anchored root covers every block under it")
	o.vouched(150)
	require.Equal(t, 50.0, matched())

	o.stop(151)
	require.Zero(t, matched(), "a demotion left the run standing")
	o.vouched(160)
	require.Zero(t, matched(), "a broken run resumed without a new proof")

	o.start(200)
	o.done(201, true, 0)
	o.done(202, true, 3)
	require.Zero(t, matched(), "a mismatch left the run standing")
	o.done(203, true, 0)
	require.Zero(t, matched())
}
