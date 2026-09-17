// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cadence is a function of the block index, so every node activates on the
// same blocks. That is what makes "two of us send" mean two NODES rather than
// two per-node timers that happen to overlap — which is what the jitter and
// back-off it replaces were trying to approximate.
func TestHealActivates_IsTheSameOnEveryNode(t *testing.T) {
	var on, off int
	for i := uint64(0); i < 1000; i++ {
		if healActivates(i) {
			on++
		} else {
			off++
		}
	}
	assert.Equal(t, 1000/healCadence, on, "one activation every healCadence blocks")
	assert.Greater(t, off, on, "and not every block: a request's answer takes blocks to come back")
}

// healers builds N conductors sharing one validator set, as N validators of a
// partition would.
func TestSelection_WouldStarveAnAnchorQuorum(t *testing.T) {
	for _, n := range []int{4, 6, 10, 16} {
		threshold := n*2/3 + 1 // what an anchor needs
		require.Greater(t, threshold, sendersPerActivation,
			"with %d validators an anchor needs %d signatures; %d senders can never reach it, "+
				"which is why the anchor push is not selected", n, threshold, sendersPerActivation)
	}
}
