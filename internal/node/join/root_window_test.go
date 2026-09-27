// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestTheRootWatchReadsTheWindowTheJoinLeft — #4452. A node that joined holds
// the ledger's bpt chain and its index from a recent window until the
// backfill brings the rest, and the root watch -- the restarted node's
// meeting point with its peers -- reads its root after each block executed
// since the handoff from them. It must find it there. Before, the search for
// the block began at the index chain's first entry, which the node does not
// hold, and the NotFound it met read as "no block recorded after it yet": the
// watch never compared a block, and said nothing.
func TestTheRootWatchReadsTheWindowTheJoinLeft(t *testing.T) {
	const n, window = 2000, 1024
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// The peer's: the root after block i at entry i, indexed at block i+1.
	whole := batch.Account(protocol.AccountUrl("whole")).BptChain()
	var roots, index [][]byte
	for i := 0; i < n; i++ {
		root := hashOf(i)
		e, err := (&protocol.IndexEntry{Source: uint64(i), BlockIndex: uint64(i + 1)}).MarshalBinary()
		require.NoError(t, err)
		roots, index = append(roots, root), append(index, e)
		require.NoError(t, whole.Inner().AddEntry(root, false))
		require.NoError(t, whole.Index().Inner().AddEntry(e, false))
	}

	// The node's: each chain from the window, as the join takes it.
	held := batch.Account(protocol.AccountUrl("held")).BptChain()
	fromWindow := func(src, dst *database.MerkleManager, entries [][]byte) {
		st, err := src.StateAt(window - 1)
		require.NoError(t, err)
		st.HashList = nil
		require.NoError(t, dst.States(window-1).Put(st.Copy()))
		for _, e := range entries[window:] {
			require.NoError(t, dst.PutBelow(st, e, false))
		}
		head, err := src.Head().Get()
		require.NoError(t, err)
		require.NoError(t, dst.RestoreHead(head, st.HashList))
		_, err = dst.Entry(0)
		require.Error(t, err, "precondition: the window holds the chain's first entry")
	}
	fromWindow(whole.Inner(), held.Inner(), roots)
	fromWindow(whole.Index().Inner(), held.Index().Inner(), index)

	for _, b := range []uint64{1500, 1900, n - 2} {
		got, ok, err := rootAfter(held, b)
		require.NoError(t, err)
		require.True(t, ok, "the root after block %d is held and was not found", b)
		require.Equal(t, [32]byte(roots[b]), got)
	}
}
