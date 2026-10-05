// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestASlowPullRoundSaysWhereItsTimeWent — #4524: a round within slowRound
// says nothing, and a slower one says, in one line, how long each phase took.
func TestASlowPullRoundSaysWhereItsTimeWent(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, nil))

	fast := newRoundTimer()
	fast.mark("anchors")
	fast.report(log, protocol.PartitionUrl("BVN2"))
	require.Empty(t, out.String(), "a fast round spoke")

	slow := newRoundTimer()
	slow.start = slow.start.Add(-9 * time.Minute)
	slow.last = slow.start
	slow.last = slow.last.Add(8 * time.Minute)
	slow.args = append(slow.args, "anchors", 8*time.Minute)
	slow.mark("records")
	slow.report(log, protocol.PartitionUrl("BVN2"))
	line := out.String()
	require.Contains(t, line, "A pull round was slow")
	require.Contains(t, line, "anchors=8m0s")
	require.Contains(t, line, "records=")
	require.Contains(t, line, "total=9m")
}
