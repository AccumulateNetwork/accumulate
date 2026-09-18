// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
)

// Claim 1: Entry(i) returns whatever occupies the position, with no check
// against the head. A head moved back over live Element records therefore
// still serves the entries above it.
func Test4312_EntryServesBeneathARestoredHead(t *testing.T) {
	var rh common.RandHash
	c := testChain(begin(), 8, "node")

	var all [][]byte
	for i := 0; i < 300; i++ { // past one mark point
		h := rh.NextList()
		all = append(all, h)
		require.NoError(t, c.AddEntry(h, true))
	}

	// The node joins and takes a peer's head at a lower height. This is
	// exactly what pullChainHeads does: merkle.Chain.RestoreHead with the
	// peer's head and open mark set. Build that head from the same chain's
	// own history so the open set verifies.
	peer := testChain(begin(), 8, "peer")
	for i := 0; i < 290; i++ {
		require.NoError(t, peer.AddEntry(all[i], true))
	}
	ph, err := peer.Head().Get()
	require.NoError(t, err)
	open, err := peer.OpenSet(ph)
	require.NoError(t, err)

	require.NoError(t, c.RestoreHead(ph.Copy(), open), "RestoreHead did not refuse a non-empty chain")

	h, err := c.Head().Get()
	require.NoError(t, err)
	require.Equal(t, int64(290), h.Count, "head did move backwards")

	// Positions 290..299 are above the head. Entry() serves them anyway.
	got, err := c.Entry(295)
	require.NoError(t, err, "Entry(295) above a head of 290 should have been unreachable")
	require.Equal(t, all[295], got, "Entry served the stale element above the head")

	// And the peer, at the same height, refuses the same read.
	_, err = peer.Entry(295)
	require.Error(t, err, "the honest chain refuses; the restored one does not")
}

// Claim 2: AddEntry(hash, unique) returns nil WITHOUT appending on an index
// hit. When the index record outlives the entry it named, the node skips an
// append its peers perform and ends short: divergence, not a leak.
func Test4312_StaleIndexSkipsTheAppendAndDiverges(t *testing.T) {
	var rh common.RandHash
	node := testChain(begin(), 8, "node")
	peer := testChain(begin(), 8, "peer")

	var all [][]byte
	for i := 0; i < 300; i++ {
		h := rh.NextList()
		all = append(all, h)
		require.NoError(t, node.AddEntry(h, true))
	}
	for i := 0; i < 290; i++ {
		require.NoError(t, peer.AddEntry(all[i], true))
	}

	ph, err := peer.Head().Get()
	require.NoError(t, err)
	open, err := peer.OpenSet(ph)
	require.NoError(t, err)
	require.NoError(t, node.RestoreHead(ph.Copy(), open))

	// Both are now at 290 and agree on the anchor.
	nh, err := node.Head().Get()
	require.NoError(t, err)
	require.Equal(t, ph.Count, nh.Count)
	require.Equal(t, ph.Anchor(), nh.Anchor(), "the join landed on the peer's anchor")

	// Now both re-execute blocks 290..299 and append the same entries.
	// The account main chain is appended with unique=true
	// (internal/core/execute/v2/chain/state_state.go:143).
	for i := 290; i < 300; i++ {
		require.NoError(t, node.AddEntry(all[i], true))
		require.NoError(t, peer.AddEntry(all[i], true))
	}

	nh, err = node.Head().Get()
	require.NoError(t, err)
	ph, err = peer.Head().Get()
	require.NoError(t, err)

	t.Logf("node height=%d anchor=%x", nh.Count, nh.Anchor())
	t.Logf("peer height=%d anchor=%x", ph.Count, ph.Anchor())

	// Documenting the defect, not asserting the fix: the node is expected to
	// end short and on a different anchor.
	require.Equal(t, int64(300), ph.Count, "peer")
	require.NotEqual(t, ph.Count, nh.Count, "if these are equal the stale index did NOT mislead")
	require.NotEqual(t, ph.Anchor(), nh.Anchor(), "divergent anchor")
}

// How many appends does one stale index record cost? Every one of them.
func Test4312_HowManyAppendsAreSkipped(t *testing.T) {
	var rh common.RandHash
	node := testChain(begin(), 8, "node")
	peer := testChain(begin(), 8, "peer")

	var all [][]byte
	for i := 0; i < 300; i++ {
		h := rh.NextList()
		all = append(all, h)
		require.NoError(t, node.AddEntry(h, true))
	}
	for i := 0; i < 290; i++ {
		require.NoError(t, peer.AddEntry(all[i], true))
	}
	ph, _ := peer.Head().Get()
	open, err := peer.OpenSet(ph)
	require.NoError(t, err)
	require.NoError(t, node.RestoreHead(ph.Copy(), open))

	skipped := 0
	for i := 290; i < 300; i++ {
		before, _ := node.Head().Get()
		require.NoError(t, node.AddEntry(all[i], true))
		after, _ := node.Head().Get()
		if before.Count == after.Count {
			skipped++
		}
	}
	t.Logf("appends skipped by a stale index: %d of 10", skipped)
	require.Equal(t, 10, skipped)
}
