// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A node is synced when its overlap's perfect blocks are followed by
// SyncedHold blocks with no regression. A demotion inside the hold withdraws
// the overlap's end: the node was still syncing (Paul, 2026-10-02).
func TestTheOverlapsEndHoldsOnlyWithoutARegression(t *testing.T) {
	newOverlap := func() *overlap {
		o := &overlap{partition: protocol.PartitionUrl("BVN1"), label: "BVN1", log: slog.Default(), blocks: 2}
		o.start(10)
		o.done(11, true, 0)
		o.done(12, true, 0) // two perfect blocks: the overlap ends at 12
		require.Equal(t, uint64(12), o.endedAt, "precondition: the overlap ended")
		return o
	}

	t.Run("a regression inside the hold", func(t *testing.T) {
		o := newOverlap()
		o.executed(12 + SyncedHold - 1)
		require.Zero(t, o.syncedAt, "synced before the hold ran out")
		o.stop(12 + SyncedHold - 1)
		require.Zero(t, o.endedAt, "a regression inside the hold left the overlap's end standing")
		o.executed(12 + 5*SyncedHold)
		require.Zero(t, o.syncedAt, "synced on an overlap whose end was withdrawn")
	})

	t.Run("a clean hold", func(t *testing.T) {
		o := newOverlap()
		o.executed(12 + SyncedHold)
		require.Equal(t, uint64(12+SyncedHold), o.syncedAt, "the hold ran out clean and the node is not synced")
		o.stop(12 + SyncedHold + 1)
		require.Equal(t, uint64(12), o.endedAt, "a demotion after the hold withdrew a synced node's overlap end")
	})
}
