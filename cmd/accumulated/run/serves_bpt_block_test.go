// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A PEER SERVES THE BPT'S INTERIOR HASHES AS OF A BLOCK THE DIRECTORY
// ANCHORED, ONE STORED BLOCK AT A TIME (#4441).
//
// executor.md, "Sync", "Two mismatches" 1: when a joining node's root differs,
// it locates the difference along the tree's storage -- the peer serves, as of
// block B, the positions eight levels below a prefix, the node compares, and
// asks again under the positions that differ.
//
// Nothing is built by hand: the daemon's own start path, the real JSON-RPC
// client over HTTP, the anchors read from the Directory's pool the way the
// join reads them. And nothing the server labels is trusted: the client
// recomputes each answer's hash from its positions (slotsHash, written here
// from the tree's rule, not imported from the server), so the root's answer
// must fold to the StateTreeAnchor the Directory holds for B, each answer
// below must fold to the position the answer above it gave, and the walk must
// end at the leaf the BPT page for B names.
func TestABptBlockIsServedAsOfTheBlockTheDirectoryAnchored(t *testing.T) {
	c := clientFor(t, startNetsimAndExecute(t))
	ctx := context.Background()

	anchors := anchorsForBVN1(t, c)
	require.NotEmpty(t, anchors)
	part := protocol.PartitionUrl("BVN1")
	ledger := part.JoinPath(protocol.Ledger)

	nowRoot, err := c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{})
	require.NoError(t, err)
	require.Equal(t, nowRoot.Hash, slotsHash(nowRoot.Slots), "the current root's block does not fold to its own hash")

	served, descended := 0, 0
	for _, a := range anchors {
		root, err := c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{ForHeight: a.block})
		if err != nil {
			require.Zerof(t, served,
				"block %d's stored block was refused after an earlier block's was served: %v", a.block, err)
			t.Logf("block %d (before the node's first answerable block): %v", a.block, err)
			continue
		}

		// THE CLAIM, from the answer's content: the positions fold to the root
		// the Directory's anchor for B carries.
		require.Equalf(t, a.block, root.Block, "the answer for block %d resolved elsewhere", a.block)
		require.Equalf(t, a.stateTreeAnchor, slotsHash(root.Slots),
			"the root's positions as of block %d do not fold to that block's StateTreeAnchor", a.block)
		require.NotEqualf(t, nowRoot.Hash, a.stateTreeAnchor, "the tree has not moved since block %d; the claim is empty", a.block)

		// The ledger's leaf at B, from the page for B, is where the walk must
		// end.
		page, err := c.QueryBptPage(ctx, part, &apiv3.BptPageQuery{Count: 4096, ForHeight: a.block})
		require.NoError(t, err)
		leaf, ok := leafFor(page, ledger)
		require.True(t, ok)

		want := root.Hash
		answer := root
		var prefix []byte
		for depth := 0; ; depth++ {
			require.Lessf(t, depth, 32, "the walk to %v did not end", ledger)
			require.Equalf(t, want, slotsHash(answer.Slots),
				"block %d, prefix %x: the answer does not fold to the position the answer above it gave", a.block, prefix)
			slot := slotAt(answer, leaf.KeyHash[len(prefix)])
			require.NotNilf(t, slot, "block %d, prefix %x: no position for %v", a.block, prefix, ledger)
			if !slot.Branch {
				require.Equal(t, leaf.KeyHash, slot.KeyHash)
				require.Equalf(t, leaf.ValueHash, slot.Hash, "block %d: the walk ends at a leaf that is not %v's at that block", a.block, ledger)
				break
			}
			prefix = append(prefix, byte(slot.Index))
			want = slot.Hash
			answer, err = c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{Prefix: prefix, ForHeight: a.block})
			require.NoErrorf(t, err, "block %d, prefix %x", a.block, prefix)
		}
		t.Logf("block %d: %v's leaf reached in %d answers", a.block, ledger, len(prefix)+1)

		// And every stored block one level down folds to the position the
		// root's answer gave for it, over the wire.
		for _, s := range root.Slots {
			if !s.Branch {
				continue
			}
			below, err := c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{Prefix: []byte{byte(s.Index)}, ForHeight: a.block})
			require.NoErrorf(t, err, "block %d, prefix %02x", a.block, s.Index)
			require.Equalf(t, s.Hash, slotsHash(below.Slots),
				"block %d, prefix %02x: the answer does not fold to the root's position for it", a.block, s.Index)
			require.Equal(t, s.Hash, below.Hash)
			descended++
		}
		served++
	}
	require.NotZero(t, served, "not one anchored block's stored block could be served")
	// The netsim partition holds few enough accounts that its root block may
	// have no branch positions; the descent over the wire with many accounts
	// is TestBptBlock_LocatesAChangedAccountOverTheWire (test/e2e).
	t.Logf("served the root's stored block for %d of %d anchored blocks, and %d blocks below it", served, len(anchors), descended)
}

// A BLOCK OUTSIDE THE RETAINED WINDOW IS REFUSED, NEVER ANSWERED WRONGLY.
//
// Below the horizon the retained versions have been pruned, so what a node
// would read there is a later tree. The refusal is IncompleteChain, a
// capability limit, and names the window; a node that retains nothing refuses
// every historical block.
func TestABptBlockOutsideRetentionIsRefused(t *testing.T) {
	part := protocol.PartitionUrl("BVN1")
	ctx := context.Background()

	depth := uint64(4)
	c := clientFor(t, startNetsimAndExecuteWith(t, &depth))
	_, err := c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{})
	require.NoError(t, err, "the current tree must keep being served")

	r, err := c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{ForHeight: 1})
	require.Error(t, err, "a block far below a four-block window was answered")
	require.Nil(t, r)
	require.Equalf(t, errors.IncompleteChain, errors.Code(err), "a capability limit must be IncompleteChain, not %v: %v", errors.Code(err), err)
	require.Contains(t, err.Error(), "retained range", "the refusal does not name the window: %v", err)

	none := uint64(0)
	c = clientFor(t, startNetsimAndExecuteWith(t, &none))
	for _, h := range []uint64{1, 3, 5, 8} {
		r, err := c.QueryBptBlock(ctx, part, &apiv3.BptBlockQuery{ForHeight: h})
		require.Errorf(t, err, "block %d was answered by a node that retains nothing", h)
		require.Nil(t, r)
		// A block this node has not reached yet is NotReady; one it has is a
		// capability limit. Neither is an answer.
		require.Containsf(t, []errors.Status{errors.IncompleteChain, errors.NotReady}, errors.Code(err), "block %d: %v", h, err)
	}
}

// slotAt returns the position with the given index, or nil when it is empty.
func slotAt(r *apiv3.BptBlockRecord, index byte) *apiv3.BptBlockSlot {
	for _, s := range r.Slots {
		if s.Index == uint64(index) {
			return s
		}
	}
	return nil
}

// slotsHash folds a stored block's positions by the BPT's rule: a set bit goes
// left, two non-empty sides hash left then right, one passes its hash up.
func slotsHash(slots []*apiv3.BptBlockSlot) [32]byte {
	var h [256]*[32]byte
	for _, s := range slots {
		v := s.Hash
		h[s.Index] = &v
	}
	for n := 256; n > 1; n /= 2 {
		for i := 0; i < n/2; i++ {
			l, r := h[2*i+1], h[2*i]
			switch {
			case l != nil && r != nil:
				v := sha256.Sum256(append(append([]byte{}, l[:]...), r[:]...))
				h[i] = &v
			case l != nil:
				h[i] = l
			default:
				h[i] = r
			}
		}
	}
	if h[0] == nil {
		return [32]byte{}
	}
	return *h[0]
}
