// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The hash appended to a chain is kept with the block's record of the append,
// so the block close does not read the chain back to recover it (#4245). The
// record answers only for the index it holds: a chain appended to again
// outside the record is read back.
func TestChainUpdates_EntryHash(t *testing.T) {
	alice := url.MustParse("alice")
	var a, b ChainUpdates
	require.NoError(t, a.DidAddChainEntry(nil, alice, "main", protocol.ChainTypeTransaction, []byte{1}, 7, 0, 0))
	require.NoError(t, b.DidAddChainEntry(nil, alice, "main", protocol.ChainTypeTransaction, []byte{2}, 8, 0, 0))
	require.NoError(t, b.DidAddChainEntry(nil, alice, "scratch", protocol.ChainTypeTransaction, []byte{3}, 0, 0, 0))

	h, ok := a.EntryHash(alice, "main", 7)
	require.True(t, ok)
	require.Equal(t, []byte{1}, h)
	_, ok = a.EntryHash(alice, "main", 8)
	require.False(t, ok, "an index the record did not append is not answered")
	_, ok = a.EntryHash(alice, "scratch", 0)
	require.False(t, ok, "a chain the record did not append to is not answered")

	// Merging keeps the later append per chain, whichever order the shards
	// merge in
	a.Merge(&b)
	h, ok = a.EntryHash(alice, "main", 8)
	require.True(t, ok)
	require.Equal(t, []byte{2}, h)
	h, ok = a.EntryHash(alice, "scratch", 0)
	require.True(t, ok)
	require.Equal(t, []byte{3}, h)

	var c ChainUpdates
	require.NoError(t, c.DidAddChainEntry(nil, alice, "main", protocol.ChainTypeTransaction, []byte{0}, 5, 0, 0))
	a.Merge(&c)
	h, _ = a.EntryHash(alice, "main", 8)
	require.Equal(t, []byte{2}, h, "an earlier index does not replace a later one")
}
