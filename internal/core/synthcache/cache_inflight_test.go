// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package synthcache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// servableAt is a node that executed block 10 and marked its synthetics
// dispatched, has since executed up to newest, and is lag blocks behind
// consensus: does it serve block 10 to a healing request?
func servableAt(lag int, newest uint64) bool {
	c := New(0)
	c.SetExecutionLagSource(func() int { return lag })
	tx := c.Begin(10)
	tx.SetBlock(&Block{Index: 10})
	tx.Commit()
	c.MarkDispatched(10, 10, nil)
	if newest > 10 {
		c.Begin(newest).Commit()
	}
	b, ok := c.Block(10)
	if !ok {
		panic("block 10 was dropped")
	}
	return c.Servable(b)
}

// Every node marks a block dispatched when it executes the block; only the
// leader sends. A node that measures the in-flight window against its own
// mark therefore answers for entries the leader has not sent yet, and the
// destination heals a delivery that is merely late (#4248). The window has
// to account for the leader, and the only lag a node can see is its own.
func TestServable_TheWindowFollowsTheSenderNotTheMark(t *testing.T) {
	const w = InFlightBlocks

	// Caught up: the mark is the send, and the window is InFlightBlocks.
	require.False(t, servableAt(0, 10+w-1), "inside the window")
	require.True(t, servableAt(0, 10+w), "the window has passed")

	// Lagging: this node executed block 10 early, but the leader is behind
	// by the partition's lag and has not sent. Serving here is the defect
	// the issue reports -- before the fix, InFlightBlocks alone made this
	// block servable and every one of the leader's blocks was healed.
	const lag = 12
	require.False(t, servableAt(lag, 10+w), "the leader has not sent yet")
	require.False(t, servableAt(lag, 10+w+lag-1), "still inside the widened window")
	require.True(t, servableAt(lag, 10+w+lag), "the widened window has passed")

	// The delay is a delay, not a refusal: a source that never catches up
	// still serves, once its own height has moved far enough past the mark.
	require.True(t, servableAt(lag, 200))
}

// The window is InFlightBlocks plus the node's lag, and nothing else.
func TestInFlightWindow_IsTheWindowPlusTheLag(t *testing.T) {
	for _, lag := range []int{0, 1, 8, 12, 40} {
		c := New(0)
		c.SetExecutionLagSource(func() int { return lag })
		require.Equal(t, uint64(InFlightBlocks+lag), c.InFlightWindow(), "lag %d", lag)
	}
}

// A cache with no lag source, and one whose source reports nonsense, behave
// as though the node were caught up: the window never narrows below
// InFlightBlocks and never depends on a negative.
func TestServable_NoLagSourceIsCaughtUp(t *testing.T) {
	c := New(0)
	require.Equal(t, uint64(0), c.ExecutionLag())
	require.Equal(t, uint64(InFlightBlocks), c.InFlightWindow())
	require.True(t, servableAt(0, 10+InFlightBlocks))

	c = New(0)
	c.SetExecutionLagSource(func() int { return -5 })
	require.Equal(t, uint64(0), c.ExecutionLag())
	require.Equal(t, uint64(InFlightBlocks), c.InFlightWindow())

	c = New(0)
	c.SetExecutionLagSource(nil)
	require.Equal(t, uint64(0), c.ExecutionLag())
}

// A block that was never dispatched is never servable, however long the node
// has been running: a lagging source answers NotReady for what it has not
// itself dispatched (#4248, option c).
func TestServable_NeverDispatchedIsNeverServable(t *testing.T) {
	c := New(0)
	tx := c.Begin(10)
	tx.SetBlock(&Block{Index: 10})
	tx.Commit()
	c.Begin(100).Commit()
	b, ok := c.Block(10)
	require.True(t, ok)
	require.False(t, b.Dispatched)
	require.False(t, c.Servable(b))
}
