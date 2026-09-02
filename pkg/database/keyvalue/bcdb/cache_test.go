// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func hashOf(i int) [32]byte {
	var h [32]byte
	binary.BigEndian.PutUint64(h[:], uint64(i))
	return h
}

// The generation swap is the eviction policy: filling the hot map makes it the
// cold one and starts a new hot map, so nothing is evicted one at a time and
// the cache is bounded at twice the limit.
func TestCache_SwapsGenerationsAtTheLimit(t *testing.T) {
	const limit = 100
	c := newRecordCache(limit)

	for i := 0; i < limit; i++ {
		c.put(hashOf(i), []byte{byte(i)})
	}
	_, _, _, gens, entries := c.stats()
	require.Equal(t, uint64(1), gens, "filling the hot map turns the generation over")
	assert.Equal(t, limit, entries, "everything is still held, now as the cold generation")

	// Everything written is still readable — the generation moved, it was not
	// discarded.
	for i := 0; i < limit; i++ {
		v, ok := c.get(hashOf(i))
		require.Truef(t, ok, "entry %d must survive the swap", i)
		assert.Equal(t, []byte{byte(i)}, v)
	}
}

// Two generations bound the cache. A third generation's worth of writes must
// not leave three generations resident.
func TestCache_IsBoundedByTwoGenerations(t *testing.T) {
	const limit = 100
	c := newRecordCache(limit)

	for i := 0; i < 5*limit; i++ {
		c.put(hashOf(i), []byte{1})
	}
	_, _, _, _, entries := c.stats()
	assert.LessOrEqual(t, entries, 2*limit,
		"the cache holds at most two generations, not one per turnover")
}

// A hit in the cold generation is promoted, so a record still in use survives
// the next turnover instead of aging out on a fixed schedule.
func TestCache_PromotesOnAColdHit(t *testing.T) {
	const limit = 10
	c := newRecordCache(limit)

	keep := hashOf(999)
	c.put(keep, []byte("keep"))
	for i := 0; i < limit; i++ { // Turn the generation over; keep goes cold
		c.put(hashOf(i), []byte{byte(i)})
	}

	v, ok := c.get(keep)
	require.True(t, ok, "a cold entry is still a hit")
	assert.Equal(t, []byte("keep"), v)
	_, _, promotions, _, _ := c.stats()
	assert.Equal(t, uint64(1), promotions)

	// It is now hot, so the NEXT turnover cannot drop it either.
	for i := limit; i < 2*limit; i++ {
		c.put(hashOf(i), []byte{byte(i)})
	}
	_, ok = c.get(keep)
	assert.True(t, ok, "a promoted entry survives the following generation")
}

// A write must drop the key. The cached records are write-once, so this should
// never fire in practice — but serving the executor a stale value is a
// consensus fault, and "should never" is not a guarantee.
func TestCache_ForgetsOnWrite(t *testing.T) {
	c := newRecordCache(100)
	h := hashOf(1)
	c.put(h, []byte("old"))
	c.forget(h)
	_, ok := c.get(h)
	assert.False(t, ok, "a key that was written must not be answered from the cache")
}

// A deletion is a zero-length value and must never be cached: a later read
// would be answered with an empty value rather than reaching the store.
func TestCache_DoesNotCacheDeletions(t *testing.T) {
	c := newRecordCache(100)
	h := hashOf(1)
	c.put(h, nil)
	c.put(h, []byte{})
	_, ok := c.get(h)
	assert.False(t, ok)
}

// What may be cached, stated as a table. The rule is narrower than
// isWriteOnce: getting THAT wrong is caught by the store refusing to
// overwrite, and getting this wrong is caught by nothing.
func TestCache_Cacheable(t *testing.T) {
	alice := url.MustParse("alice.acme")
	var hash [32]byte
	hash[0] = 1

	cases := []struct {
		ok  bool
		why string
		key *record.Key
	}{
		{true, "the URL an account is keyed by", record.NewKey("Account", alice, "Url")},
		{true, "a message is its own hash", record.NewKey("Message", hash, "Main")},
		{true, "a transaction is its own hash", record.NewKey("Transaction", hash, "Main")},
		{true, "an anchor chain entry is a position in a log",
			record.NewKey("Account", alice, "AnchorChain", "bvn1", "Root", "Element", uint64(3))},
		{true, "and so is its mark point",
			record.NewKey("Account", alice, "AnchorChain", "bvn1", "Root", "States", uint64(3))},
		{true, "the anchor sequence chain likewise",
			record.NewKey("Account", alice, "AnchorSequenceChain", "Element", uint64(7))},

		{false, "an account's main state is the thing that changes",
			record.NewKey("Account", alice, "Main")},
		{false, "a transaction's status changes", record.NewKey("Transaction", hash, "Status")},
		{false, "a chain's head moves",
			record.NewKey("Account", alice, "AnchorChain", "bvn1", "Root", "Head")},
		{false, "a NON-anchor chain's elements are left alone",
			record.NewKey("Account", alice, "MainChain", "Element", uint64(3))},
		{false, "the pending set changes", record.NewKey("Account", alice, "Pending")},
		{false, "a BSN summary is not in the asked-for set",
			record.NewKey("Summary", hash, "Main")},
	}
	for _, c := range cases {
		assert.Equalf(t, c.ok, cacheable(c.key), "%v: %s", c.key, c.why)
	}
}

// End to end: a record read twice is answered from the cache the second time,
// and a rewrite is not answered from it at all.
func TestCache_ServesRepeatedReadsAndNotStaleOnes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	defer db.Close()

	key := record.NewKey("Account", url.MustParse("alice.acme"), "Url")
	b := db.Begin(nil, true)
	require.NoError(t, b.Put(key, []byte("alice.acme")))
	require.NoError(t, b.Commit())

	// First read reaches the store and fills the cache; the second is a hit.
	for i := 0; i < 2; i++ {
		r := db.Begin(nil, false)
		v, err := r.Get(key)
		require.NoError(t, err)
		require.Equal(t, []byte("alice.acme"), v)
		r.Discard()
	}
	hits, _, _, _, _ := db.cache.stats()
	assert.NotZero(t, hits, "the second read must come from the cache")

	// A rewrite must not be shadowed by what the cache holds. This record is
	// write-once in practice, which is why it may be cached at all; the point
	// is that the cache does not make a stale answer possible if it is not.
	b = db.Begin(nil, true)
	require.NoError(t, b.Put(key, []byte("rewritten")))
	require.NoError(t, b.Commit())

	r := db.Begin(nil, false)
	defer r.Discard()
	v, err := r.Get(key)
	require.NoError(t, err)
	assert.Equal(t, []byte("rewritten"), v, "a write must not be answered from the cache")
}

// The cache must not break isolation: a batch open at an older version must
// not see a record committed after it began.
func TestCache_DoesNotLeakNewerVersionsToOpenReaders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	defer db.Close()

	key := func(i int) *record.Key {
		return record.NewKey("Account", url.MustParse(fmt.Sprintf("acc%d.acme", i)), "Url")
	}

	b := db.Begin(nil, true)
	require.NoError(t, b.Put(key(1), []byte("one")))
	require.NoError(t, b.Commit())

	// Open a reader, then commit something new while it is open.
	old := db.Begin(nil, false)
	defer old.Discard()

	b = db.Begin(nil, true)
	require.NoError(t, b.Put(key(2), []byte("two")))
	require.NoError(t, b.Commit())

	_, err = old.Get(key(2))
	require.Error(t, err, "a reader must not see a record committed after it began")

	// And the record it should see is still there.
	v, err := old.Get(key(1))
	require.NoError(t, err)
	assert.Equal(t, []byte("one"), v)
}
