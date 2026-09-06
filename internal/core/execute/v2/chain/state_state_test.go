// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// Per-message states merge in hash order, not execution order, so the
// segment of a chain two messages appended to in one block can arrive with
// its later span first. Whichever order they merge in, the block's segment
// starts at the block's first append and holds every element in chain order
// — a receipt for the first entry must be buildable from it.
func TestChainUpdates_MergeSegmentsInEitherOrder(t *testing.T) {
	h := func(i byte) []byte { v := sha256.Sum256([]byte{i}); return v[:] }
	first := func() *ChainUpdates {
		return &ChainUpdates{Segments: map[string]*merkle.Segment{"k": {First: 0, Before: new(merkle.State), Elements: [][]byte{h(0)}}}}
	}
	second := func() *ChainUpdates {
		before := new(merkle.State)
		before.AddEntry(h(0))
		return &ChainUpdates{Segments: map[string]*merkle.Segment{"k": {First: 1, Before: before, Elements: [][]byte{h(1)}}}}
	}
	check := func(t *testing.T, c *ChainUpdates) {
		seg := c.Segments["k"]
		require.EqualValues(t, 0, seg.First)
		require.EqualValues(t, 1, seg.Last())
		require.Equal(t, [][]byte{h(0), h(1)}, seg.Elements)
		require.EqualValues(t, 0, seg.Before.Count, "the state before the block's first append")
		r, err := seg.Receipt(0, 1)
		require.NoError(t, err)
		require.True(t, r.Validate(nil))
	}

	c := new(ChainUpdates)
	c.Merge(first())
	c.Merge(second())
	check(t, c)

	c = new(ChainUpdates)
	c.Merge(second())
	c.Merge(first())
	check(t, c)
}
