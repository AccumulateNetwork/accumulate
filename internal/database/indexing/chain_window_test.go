// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSearchingAnIndexChainHeldFromAWindow — #4452. A node that joined holds
// the ledger's and the pool's index chains from a recent window until the
// backfill brings the rest. A search for a recent block reads only near the
// start it is given, and finds it; a search whose answer lies below what is
// held says so (ErrNotHeld) rather than failing on the first entry it cannot
// read, or answering NotFound for a block that exists.
func TestSearchingAnIndexChainHeldFromAWindow(t *testing.T) {
	const n, window = 2000, 1024
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	whole := batch.Account(protocol.AccountUrl("whole")).MainChain().Index()
	var entries [][]byte
	for i := 0; i < n; i++ {
		b, err := (&protocol.IndexEntry{Source: uint64(3 * i), BlockIndex: uint64(i + 1)}).MarshalBinary()
		require.NoError(t, err)
		entries = append(entries, b)
		require.NoError(t, whole.Inner().AddEntry(b, false))
	}

	// Held from the window: the state at its first mark point, the entries
	// from there.
	held := batch.Account(protocol.AccountUrl("held")).MainChain().Index()
	st, err := whole.Inner().StateAt(window - 1)
	require.NoError(t, err)
	st.HashList = nil
	require.NoError(t, held.Inner().States(window-1).Put(st.Copy()))
	for _, e := range entries[window:] {
		require.NoError(t, held.Inner().PutBelow(st, e, false))
	}
	head, err := whole.Inner().Head().Get()
	require.NoError(t, err)
	require.NoError(t, held.Inner().RestoreHead(head, st.HashList))
	c, err := held.Get()
	require.NoError(t, err)
	top := uint64(n - 1)

	// A recent block: found.
	i, e, err := SearchIndexChain(c, top, MatchExact, SearchIndexChainByBlock(1501))
	require.NoError(t, err)
	require.Equal(t, uint64(1500), i)
	require.Equal(t, uint64(1501), e.BlockIndex)
	_, e, err = SearchIndexChain(c, top, MatchAfter, SearchIndexChainByBlock(1900))
	require.NoError(t, err)
	require.Equal(t, uint64(1900), e.BlockIndex)

	// A block below the window: not held, and not NotFound.
	for _, b := range []uint64{window, 500, 1} {
		_, _, err = SearchIndexChain(c, top, MatchExact, SearchIndexChainByBlock(b))
		require.True(t, stderrors.Is(err, ErrNotHeld), "block %d: %v", b, err)
	}
}
