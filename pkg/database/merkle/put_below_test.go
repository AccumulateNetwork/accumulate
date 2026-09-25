// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPutBelow_WritesWhatAddEntryWrites — #4446. A pull takes a chain's head
// from a peer and streams the entries under it a page at a time, without
// replaying them through the head. What it writes must be what appending them
// wrote on the peer: the same elements, indices, mark points and stored
// intermediates, so a receipt taken from it is the peer's.
func TestPutBelow_WritesWhatAddEntryWrites(t *testing.T) {
	const n = 1000 // not a mark point: an open set is left
	peer := testChain(begin(), 4, "test")
	for i := 0; i < n; i++ {
		d := sha256.Sum256([]byte(fmt.Sprint(i)))
		require.NoError(t, peer.AddEntry(d[:], false))
	}
	head, err := peer.Head().Get()
	require.NoError(t, err)

	node := testChain(begin(), 4, "test")
	st := new(State)
	for i := int64(0); i < n; i++ {
		h, err := peer.Entry(i)
		require.NoError(t, err)
		require.NoError(t, node.PutBelow(st, h, false))
	}
	require.Equal(t, head.Anchor(), st.Anchor(), "the entries do not reproduce the head")
	require.NoError(t, node.RestoreHead(head, st.HashList))

	for i := int64(0); i < n; i++ {
		want, err := peer.Entry(i)
		require.NoError(t, err)
		got, err := node.Entry(i)
		require.NoError(t, err)
		require.Equal(t, want, got, "element %d", i)
		j, err := node.ElementIndex(got).Get()
		require.NoError(t, err)
		require.Equal(t, uint64(i), j, "index of %d", i)
		for h := uint64(1); h <= uint64(trailingOnes(uint64(i))); h++ {
			w, err := peer.Intermediate(uint64(i), h).Get()
			require.NoError(t, err)
			g, err := node.Intermediate(uint64(i), h).Get()
			require.NoError(t, err, "intermediate %d/%d", i, h)
			require.Equal(t, w, g, "intermediate %d/%d", i, h)
		}
		if (i+1)&peer.markMask == 0 {
			w, err := peer.States(uint64(i)).Get()
			require.NoError(t, err)
			g, err := node.States(uint64(i)).Get()
			require.NoError(t, err, "mark point %d", i)
			require.True(t, w.Equal(g), "mark point %d", i)
		}
	}
	for _, span := range [][2]int64{{0, n - 1}, {17, 700}, {512, 999}} {
		w, err := peer.Receipt(span[0], span[1])
		require.NoError(t, err)
		g, err := node.Receipt(span[0], span[1])
		require.NoError(t, err)
		require.True(t, w.Equal(g), "receipt %v", span)
	}
	require.NoError(t, node.AddEntry(make([]byte, 32), false), "the node cannot append to what it took")
}
