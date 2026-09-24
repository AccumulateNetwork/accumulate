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
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
)

// retake replaces dst with the entries of src the way the join's pull takes a
// chain again whole (internal/core/bootstrap/pull, pullChain with whole):
// the head is started over and every entry is replayed with unique false.
func retake(t *testing.T, dst, src *Chain) {
	t.Helper()
	head, err := src.Head().Get()
	require.NoError(t, err)
	entries, err := src.Entries(0, head.Count)
	require.NoError(t, err)
	require.NoError(t, dst.RestoreHead(new(State), nil))
	for _, e := range entries {
		require.NoError(t, dst.AddEntry(e, false))
	}
	got, err := dst.Head().Get()
	require.NoError(t, err)
	require.Equal(t, head.Anchor(), got.Anchor(), "precondition: the retake reproduces the peer's chain")
}

func requireSameChain(t *testing.T, want, got *Chain, msg string) {
	t.Helper()
	wh, err := want.Head().Get()
	require.NoError(t, err)
	gh, err := got.Head().Get()
	require.NoError(t, err)
	require.Equal(t, wh.Count, gh.Count, msg)
	require.Equal(t, wh.Anchor(), gh.Anchor(), msg)
}

// TestARetakenChainAppendsAHashOnlyItsWrongGrowthHeld — #4444. A node that
// grew a chain wrongly with X holds X's element index. Replacing the chain
// with the peers' (executor.md, Sync, "Two mismatches" 2: a chain grown
// wrongly is replaced, not appended to) rewrites every position, but the
// index of X still names the position X was last written at, which now holds
// the peers' entry — or lies past the new head, when the node's wrong chain
// was the longer. A later honest append of X with unique set must append, as
// it does on the peers; skipping it on that index leaves the node one entry
// short, on a different anchor, for ever.
func TestARetakenChainAppendsAHashOnlyItsWrongGrowthHeld(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wrong, own int // entries the node and the peer append past the common prefix
	}{
		{"node-shorter", 2, 5},
		{"node-longer", 5, 2}, // a repair replaces the node's longer chain too
		{"x-at-the-new-head", 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rh common.RandHash
			peer := testChain(begin(), 2, "peer") // 4 hashes a mark set: the retake crosses mark points
			node := testChain(begin(), 2, "node")
			for i := 0; i < 6; i++ {
				h := rh.NextList()
				require.NoError(t, peer.AddEntry(h, true))
				require.NoError(t, node.AddEntry(h, true))
			}

			// The node's wrong growth ends in X; the peer's is its own.
			var x []byte
			for i := 0; i < tc.wrong; i++ {
				x = rh.NextList()
				require.NoError(t, node.AddEntry(x, true))
			}
			for i := 0; i < tc.own; i++ {
				require.NoError(t, peer.AddEntry(rh.NextList(), true))
			}

			retake(t, node, peer)

			// X arrives honestly, on every node.
			require.NoError(t, peer.AddEntry(x, true))
			require.NoError(t, node.AddEntry(x, true))
			requireSameChain(t, peer, node, "the retaken node skipped an honest append of a hash only its wrong chain held")

			i, err := node.IndexOf(x)
			require.NoError(t, err)
			e, err := node.Element(uint64(i)).Get()
			require.NoError(t, err)
			require.Equal(t, x, e, "the index of X does not name X")
		})
	}
}

// TestAUniqueAppendOfAHeldHashIsStillSkipped — the element index is refuted
// only by the element it names. A hash the chain holds is skipped with
// unique set, and reported to OnDuplicate, as before; one the peers' chain
// holds after a retake is skipped too, since the replay indexed it where it
// now is. A chain appended without unique keeps every repeat.
func TestAUniqueAppendOfAHeldHashIsStillSkipped(t *testing.T) {
	var dups int
	OnDuplicate = func(*database.Key, bool) { dups++ }
	defer func() { OnDuplicate = nil }()

	var rh common.RandHash
	peer := testChain(begin(), 2, "peer")
	node := testChain(begin(), 2, "node")
	var held [][]byte
	for i := 0; i < 11; i++ {
		h := rh.NextList()
		held = append(held, h)
		require.NoError(t, peer.AddEntry(h, true))
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, node.AddEntry(rh.NextList(), true))
	}
	retake(t, node, peer)

	for _, c := range []*Chain{peer, node} {
		before, err := c.Head().Get()
		require.NoError(t, err)
		count := before.Count
		for _, h := range held {
			require.NoError(t, c.AddEntry(h, true))
		}
		after, err := c.Head().Get()
		require.NoError(t, err)
		require.Equal(t, count, after.Count, "a unique append of a hash the chain holds was appended")
	}
	require.Equal(t, 2*len(held), dups, "a skipped duplicate was not reported")

	// Without unique, a repeat is appended and the index names it last.
	c := testChain(begin(), 2, "repeats")
	h := rh.NextList()
	require.NoError(t, c.AddEntry(h, false))
	require.NoError(t, c.AddEntry(rh.NextList(), false))
	require.NoError(t, c.AddEntry(h, false))
	head, err := c.Head().Get()
	require.NoError(t, err)
	require.Equal(t, int64(3), head.Count)
	i, err := c.IndexOf(h)
	require.NoError(t, err)
	require.Equal(t, int64(2), i, "the index does not name the last write")
}

// TestAnIndexWhoseElementIsNotHeldStillSkips — only an element that is held
// and is another hash refutes the index. A chain restored from a head holds
// no elements below its open mark set, and a windowed store answers an old
// element as absent; neither is evidence that the hash is not on the chain,
// and appending on it would make a node's chain depend on what its store
// still reads.
func TestAnIndexWhoseElementIsNotHeldStillSkips(t *testing.T) {
	var rh common.RandHash
	src := testChain(begin(), 2, "src")
	var first []byte
	for i := 0; i < 10; i++ {
		h := rh.NextList()
		if i == 0 {
			first = h
		}
		require.NoError(t, src.AddEntry(h, true))
	}
	head, err := src.Head().Get()
	require.NoError(t, err)
	open, err := src.OpenSet(head)
	require.NoError(t, err)

	dst := testChain(begin(), 2, "dst")
	require.NoError(t, dst.RestoreHead(head.Copy(), open))
	require.NoError(t, dst.ElementIndex(first).Put(0)) // rebuilt, say; Element(0) is not held
	_, err = dst.Element(0).Get()
	require.Error(t, err, "precondition: the element is not held")

	require.NoError(t, dst.AddEntry(first, true))
	got, err := dst.Head().Get()
	require.NoError(t, err)
	require.Equal(t, head.Count, got.Count, "an index whose element is not held was taken as refuted")
}
