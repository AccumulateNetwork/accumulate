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

// Spans of an anchor chain fold in chain order: messages execute in the
// order staging released them and a bundle folds its states in that order.
// A span that continues the block's segment joins it, and a receipt for any
// entry builds from the result. A span that does not continue it is the
// #4279 failure -- per-message states were folded in hash order, and the
// third of three anchors from one partition was dropped -- and is refused
// out loud rather than dropped.
func TestChainUpdates_MergeSegmentsInChainOrder(t *testing.T) {
	const n = 3
	h := func(i byte) []byte { v := sha256.Sum256([]byte{i}); return v[:] }
	span := func(i int) *ChainUpdates {
		before := new(merkle.State)
		for j := 0; j < i; j++ {
			before.AddEntry(h(byte(j)))
		}
		return &ChainUpdates{Segments: map[string]*merkle.Segment{"k": {First: int64(i), Before: before, Elements: [][]byte{h(byte(i))}}}}
	}

	c := new(ChainUpdates)
	for i := 0; i < n; i++ {
		c.Merge(span(i))
	}
	require.NoError(t, c.SegmentError("k"))
	seg := c.Segments["k"]
	require.EqualValues(t, 0, seg.First)
	require.EqualValues(t, n-1, seg.Last())
	require.Equal(t, [][]byte{h(0), h(1), h(2)}, seg.Elements)
	require.EqualValues(t, 0, seg.Before.Count, "the state before the block's first append")
	for i := 0; i < n; i++ {
		r, err := seg.Receipt(int64(i), n-1)
		require.NoError(t, err, "entry %d", i)
		require.True(t, r.Validate(nil), "entry %d", i)
	}

	// The order that broke run 20260915T042428Z: the last span first. Not
	// a hole in the segment -- an error that names the span.
	c = new(ChainUpdates)
	c.Merge(span(2))
	c.Merge(span(0))
	c.Merge(span(1))
	err := c.SegmentError("k")
	require.Error(t, err)
	require.Contains(t, err.Error(), "span [0, 0] folded after [2, 2]")
	require.NoError(t, c.SegmentError("another-chain"),
		"the fault belongs to the chain with the hole, not to the block")

	// The error survives folding into a parent
	parent := new(ChainUpdates)
	parent.Merge(c)
	require.Error(t, parent.SegmentError("k"))
}
