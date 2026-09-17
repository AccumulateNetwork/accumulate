// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func blockEntries(n int) []*protocol.BlockEntry {
	entries := make([]*protocol.BlockEntry, n)
	for i := range entries {
		// Two chains per account, so the account compare is exercised
		entries[i] = &protocol.BlockEntry{
			Account: url.MustParse(fmt.Sprintf("acc%d.acme", i/2)),
			Chain:   []string{"main", "signature"}[i%2],
			Index:   uint64(i),
		}
	}
	return entries
}

// AddChainEntry2 asks whether the block already holds an entry for a chain.
// That used to scan every entry with a URL compare -- O(E²) per block, 1-3 M
// compares at 500 tps (#4226). The entries are indexed by (account, chain);
// the slice stays the source of truth, so the index survives the block
// rebuilding and sorting the slice under it.
func TestChainUpdatesFindEntriesWithoutScanning(t *testing.T) {
	var u ChainUpdates
	entries := blockEntries(2000)
	for _, e := range entries {
		u.DidUpdateChain(e)
	}
	for _, e := range entries {
		got, ok := u.entryFor(e.Account, e.Chain)
		require.True(t, ok)
		require.Same(t, e, got)
	}
	_, ok := u.entryFor(url.MustParse("nobody.acme"), "main")
	require.False(t, ok)

	// The block rebuilds the slice directly and sorts it
	// (enumerateModifiedChains): the index is not consulted blind.
	u.Entries = nil
	rebuilt := blockEntries(1500)
	u.Entries = append(u.Entries, rebuilt...)
	sort.Slice(u.Entries, func(i, j int) bool { return u.Entries[i].Compare(u.Entries[j]) < 0 })
	for _, e := range rebuilt {
		got, ok := u.entryFor(e.Account, e.Chain)
		require.True(t, ok)
		require.Same(t, e, got)
	}
	_, ok = u.entryFor(entries[1999].Account, entries[1999].Chain)
	require.False(t, ok, "an entry the rebuilt slice does not hold is not found")

	// Appended after the rebuild, through the recording path
	extra := &protocol.BlockEntry{Account: url.MustParse("extra.acme"), Chain: "main", Index: 7}
	u.DidUpdateChain(extra)
	got, ok := u.entryFor(extra.Account, extra.Chain)
	require.True(t, ok)
	require.Same(t, extra, got)

	// The first entry for a chain wins, as the scan did
	var v ChainUpdates
	first := &protocol.BlockEntry{Account: url.MustParse("a.acme"), Chain: "main", Index: 1}
	v.DidUpdateChain(first)
	v.DidUpdateChain(&protocol.BlockEntry{Account: url.MustParse("A.ACME"), Chain: "main", Index: 2})
	got, ok = v.entryFor(url.MustParse("a.acme"), "main")
	require.True(t, ok)
	require.Same(t, first, got)

	// Merging keeps the merged-into index current
	var w ChainUpdates
	w.Merge(&v)
	got, ok = w.entryFor(url.MustParse("a.acme"), "main")
	require.True(t, ok)
	require.Same(t, first, got)
}

// One lookup per entry of a 2000-entry block. Linear in E overall, so
// ns/op here is flat as E grows; the scan it replaces was E compares per
// lookup.
func BenchmarkChainUpdatesEntryFor(b *testing.B) {
	for _, n := range []int{200, 2000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			var u ChainUpdates
			entries := blockEntries(n)
			for _, e := range entries {
				u.DidUpdateChain(e)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e := entries[i%n]
				if _, ok := u.entryFor(e.Account, e.Chain); !ok {
					b.Fatal("missing")
				}
			}
		})
	}
}

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
