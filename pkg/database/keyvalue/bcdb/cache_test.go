// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"crypto/sha256"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func acct(path ...string) *url.URL { return protocol.PartitionUrl("BVN0").JoinPath(path...) }

// The classification names shapes and nothing else. What it does not name
// is read from the store every time, which is the safe direction: a cache
// over a MUTABLE record would be a correctness bug, not a slow path.
func TestCacheKind_NamesOnlyImmutableShapes(t *testing.T) {
	synth := acct(protocol.Synthetic)
	anchors := acct(protocol.AnchorPool)
	alice := protocol.AccountUrl("alice", "tokens")

	cases := []struct {
		want cacheKind
		key  *record.Key
		why  string
	}{
		{cacheURL, record.NewKey("Account", alice, "Url"), "written once at account creation, read on every touch"},
		{cacheURL, record.NewKey("Account", synth, "Url"), "any account's URL"},

		{cacheChain, record.NewKey("Account", synth, "MainChain", "Element", uint64(5)), "a position in an append-only log"},
		{cacheChain, record.NewKey("Account", synth, "MainChain", "ElementIndex", [32]byte{1}), "where an element landed"},
		{cacheChain, record.NewKey("Account", anchors, "AnchorSequenceChain", "States", uint64(9)), "a mark point"},

		// Not cached, and each for a reason that matters.
		{cacheNone, record.NewKey("Account", alice, "MainChain", "Element", uint64(5)), "a chain, but not synthetic or anchor"},
		{cacheNone, record.NewKey("Account", synth, "MainChain", "Head"), "the chain's END moves with every append"},
		{cacheNone, record.NewKey("Account", synth, "Main"), "an account's main state is what changes"},
		{cacheNone, record.NewKey("Account", synth, "Pending"), "a mutable set"},
		{cacheNone, record.NewKey("Message", [32]byte{2}, "Main"), "immutable, but read once -- not worth a cache slot"},
		{cacheNone, record.NewKey("Transaction", [32]byte{3}, "Status"), "MUTABLE: the record of what has happened since"},
	}

	for _, c := range cases {
		require.Equalf(t, c.want, cacheKindOf(c.key), "%v: %s", c.key, c.why)
	}
}

// A URL is served from the DYNAMIC layer. In the permanent layer it would
// age out of the read window almost immediately -- written once at account
// creation, read for the life of the account -- and every read after that
// would be a history walk.
func TestCache_UrlIsServedFromDynamic(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()

	alice := protocol.AccountUrl("alice", "tokens")
	k := record.NewKey("Account", alice, "Url")
	put(t, d, k, "acc://alice.acme/tokens")

	// It is not in the permanent layer at all.
	h := d.prefix.AppendKey(k).Hash()
	_, err = d.kv.GetPerm(h)
	require.Error(t, err, "a URL in the permanent layer is the placement bug this avoids")
	_, err = d.kv.GetDyna(h)
	require.NoError(t, err, "it belongs in the dynamic layer")

	// The write warmed the cache, so even the first read is a hit: an
	// account's URL is written once, at creation, and the very next
	// thing that happens to that account reads it back.
	require.Equal(t, "acc://alice.acme/tokens", get(t, d, k))
	get(t, d, k)
	hits, misses, entries := d.urls.stats()
	require.Equal(t, uint64(2), hits)
	require.Zero(t, misses, "written through on commit, so never fetched")
	require.Equal(t, 1, entries)
}

// Chain records are served from the PERMANENT layer, which is correct for
// them: a chain entry is a fact about a position in a log and never moves.
func TestCache_ChainsAreServedFromPermanent(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()

	k := record.NewKey("Account", acct(protocol.Synthetic), "MainChain", "Element", uint64(7))
	put(t, d, k, "element")

	h := d.prefix.AppendKey(k).Hash()
	_, err = d.kv.GetPerm(h)
	require.NoError(t, err, "chain elements are write-once and belong in perm")

	require.Equal(t, "element", get(t, d, k))
	get(t, d, k)
	hits, _, _ := d.chains.stats()
	require.Equal(t, uint64(2), hits)

	// And the two caches are separate: a chain read does not touch the
	// URL cache, so neither can evict the other.
	_, _, urlEntries := d.urls.stats()
	require.Zero(t, urlEntries)
}

// A write wins over anything cached.
//
// These caches hold records that cannot change, so this should never
// happen -- but a cache that goes stale when that assumption is broken
// turns a wrong classification into wrong DATA, served forever and
// silently. The commit writes through, so the caches cannot disagree with
// the store whatever the shape rules do.
func TestCache_StagedWritesWin(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()

	k := record.NewKey("Account", protocol.AccountUrl("bob"), "Url")
	put(t, d, k, "first")
	get(t, d, k) // cache it
	put(t, d, k, "second")

	require.Equal(t, "second", get(t, d, k),
		"the newest write wins over anything cached")
}

// The cache holds two generations and drops the oldest in one step, so it
// is bounded without per-entry eviction bookkeeping.
func TestImmutableCache_TwoGenerations(t *testing.T) {
	c := newImmutableCache(4)

	for i := 0; i < 4; i++ {
		c.put(sha256.Sum256([]byte{byte(i)}), []byte{byte(i)})
	}
	_, _, n := c.stats()
	require.Equal(t, 4, n)

	// The fifth swaps: the first four move to cold, and are still found.
	c.put(sha256.Sum256([]byte{9}), []byte{9})
	v, ok := c.get(sha256.Sum256([]byte{0}))
	require.True(t, ok, "one generation back is still a hit")
	require.Equal(t, []byte{0}, v)

	// Filling hot again drops what was never re-read.
	for i := 10; i < 15; i++ {
		c.put(sha256.Sum256([]byte{byte(i)}), []byte{byte(i)})
	}
	_, ok = c.get(sha256.Sum256([]byte{1}))
	require.False(t, ok, "never re-read, so it fell out after two generations")

	_, ok = c.get(sha256.Sum256([]byte{0}))
	require.True(t, ok, "promoted on its cold hit, so it survived")
}
