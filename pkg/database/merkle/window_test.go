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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// TestAChainHeldFromAWindowSaysWhatItDoesNotHold — #4452. A join takes a
// chain from a window: the state at the window's first mark point, written as
// the mark point that closes the set below with none of that set's hashes,
// and the entries from there. What lies below is not held until backfill
// brings it, and a read of it says NotFound -- the answer the executor's
// readers take as the start of what the node holds -- rather than a corrupt
// record, and never a state built from hashes that are not there.
func TestAChainHeldFromAWindowSaysWhatItDoesNotHold(t *testing.T) {
	const n, window = 1000, 512 // mark power 8: sets of 256
	var rh common.RandHash
	store := begin()
	whole := testChain(store, 8, "whole")
	require.NoError(t, whole.Head().Put(new(State)))
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := rh.NextList()
		hashes = append(hashes, h)
		require.NoError(t, whole.AddEntry(h, false))
	}

	// The window: the state at 512 from the whole chain, then the entries.
	held := testChain(store, 8, "held")
	st, err := whole.StateAt(window - 1)
	require.NoError(t, err)
	st.HashList = nil
	require.NoError(t, held.States(window-1).Put(st.Copy()))
	for _, h := range hashes[window:] {
		require.NoError(t, held.PutBelow(st, h, false))
	}
	head, err := whole.Head().Get()
	require.NoError(t, err)
	require.NoError(t, held.RestoreHead(head, st.HashList))

	// Held: the window's entries and states.
	for _, i := range []int64{window, 700, n - 1} {
		got, err := held.Entry(i)
		require.NoError(t, err)
		require.Equal(t, hashes[i], got)
		want, err := whole.StateAt(i)
		require.NoError(t, err)
		gotSt, err := held.StateAt(i)
		require.NoError(t, err)
		require.Equal(t, want.Anchor(), gotSt.Anchor(), "state at %d", i)
	}

	// Not held: the set just below the window, and deeper.
	for _, i := range []int64{window - 1, 300, 10} {
		_, err := held.Entry(i)
		require.Error(t, err)
		require.True(t, errors.Is(err, errors.NotFound), "entry %d: %v", i, err)
	}

	// A backfill part way: the mark point below the window's is held, the
	// set between is not. The state inside that set is not computed from
	// the hashless mark point above it.
	below, err := whole.States(255).Get()
	require.NoError(t, err)
	require.NoError(t, held.States(255).Put(below))
	_, err = held.StateAt(300)
	require.Error(t, err, "a state was built from a set whose hashes are not held")
	require.True(t, errors.Is(err, errors.NotFound), "%v", err)
}
