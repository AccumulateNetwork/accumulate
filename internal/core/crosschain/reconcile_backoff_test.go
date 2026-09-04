// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A source whose every request failed is asked again after 1, 2, 4 ... blocks,
// capped, and one success clears it (healing spec, "Sending"). Run
// 20260904T012004Z asked failing sources 9.4 million times in forty minutes.
func TestReconcileBackoff_DoublesOnFailureAndResetsOnSuccess(t *testing.T) {
	c := &Conductor{Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}}
	src := protocol.PartitionUrl("BVN2")

	require.False(t, c.sourceBackedOff(src, 100))

	c.recordSourceOutcome(src, 100, 5, 5) // everything failed
	require.True(t, c.sourceBackedOff(src, 100))
	require.False(t, c.sourceBackedOff(src, 101), "one block later it is asked again")

	c.recordSourceOutcome(src, 101, 5, 5)
	require.True(t, c.sourceBackedOff(src, 102))
	require.False(t, c.sourceBackedOff(src, 103), "two blocks the second time")

	for i := 0; i < 10; i++ {
		c.recordSourceOutcome(src, 200, 5, 5)
	}
	require.True(t, c.sourceBackedOff(src, 200+maxReconcileBackoffBlocks-1))
	require.False(t, c.sourceBackedOff(src, 200+maxReconcileBackoffBlocks), "the wait is capped")

	c.recordSourceOutcome(src, 300, 5, 4) // one answer is enough
	require.False(t, c.sourceBackedOff(src, 300))
	c.recordSourceOutcome(src, 301, 5, 5)
	require.False(t, c.sourceBackedOff(src, 302), "after a success the ladder starts over at one block")
}

// An activation that asked nothing changes nothing.
func TestReconcileBackoff_NoRequestsNoChange(t *testing.T) {
	c := &Conductor{Partition: &protocol.PartitionInfo{ID: "BVN1"}}
	src := protocol.PartitionUrl("BVN2")
	c.recordSourceOutcome(src, 100, 0, 0)
	require.False(t, c.sourceBackedOff(src, 100))
}
