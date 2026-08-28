// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package leveldb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// The write-path record cache (#4164) must be invisible except for speed:
// writable batches see latest-committed state exactly, read-only batches
// keep pure snapshot semantics, deletes are honored, and caller mutations
// cannot poison the cache.
func TestRecordCache_Semantics(t *testing.T) {
	db, err := Open(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	k := record.NewKey("account", "alice")

	// Commit v1.
	w := db.Begin(nil, true)
	require.NoError(t, w.Put(k, []byte("v1")))
	require.NoError(t, w.Commit())

	// A read-only snapshot taken NOW must keep seeing v1 forever.
	ro := db.Begin(nil, false)
	defer ro.Discard()

	// Writable read sees v1 (via cache), then overwrite to v2.
	w = db.Begin(nil, true)
	v, err := w.Get(k)
	require.NoError(t, err)
	require.Equal(t, []byte("v1"), v)
	// Mutate the returned slice — must not poison later reads.
	v[0] = 'X'
	require.NoError(t, w.Put(k, []byte("v2")))
	require.NoError(t, w.Commit())

	// New writable batch sees v2 exactly.
	w = db.Begin(nil, true)
	v, err = w.Get(k)
	require.NoError(t, err)
	require.Equal(t, []byte("v2"), v, "writable batches see latest-committed state")
	w.Discard()

	// The old read-only snapshot still sees v1 — the cache must never leak
	// newer state into an older snapshot.
	v, err = ro.Get(k)
	require.NoError(t, err)
	require.Equal(t, []byte("v1"), v, "read-only batches keep pure snapshot semantics")

	// Delete, then a writable read reports not-found (negative entry).
	w = db.Begin(nil, true)
	require.NoError(t, w.Delete(k))
	require.NoError(t, w.Commit())
	w = db.Begin(nil, true)
	_, err = w.Get(k)
	require.Error(t, err, "a deleted record must be gone on the write path")
	w.Discard()
}

// Values past the cacheable size must bypass the cache without breaking
// reads, and the byte budget must evict rather than grow.
func TestRecordCache_Bounds(t *testing.T) {
	db, err := Open(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	big := make([]byte, recordCacheMaxValue+1)
	big[0] = 7
	k := record.NewKey("big")
	w := db.Begin(nil, true)
	require.NoError(t, w.Put(k, big))
	require.NoError(t, w.Commit())

	w = db.Begin(nil, true)
	v, err := w.Get(k)
	require.NoError(t, err)
	require.Equal(t, big, v, "oversized values read correctly without being cached")
	w.Discard()

	db.records.mu.Lock()
	bytes := db.records.bytes
	db.records.mu.Unlock()
	require.LessOrEqual(t, bytes, recordCacheMaxBytes, "the cache honors its byte budget")
}
