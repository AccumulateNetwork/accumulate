// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A node restarts. The database it reopens must accept commits, and the
// second commit is the one that tells: a commit's own write-through happens
// when its batch closes, so its error surfaces on the NEXT Commit.
func TestRestart_CommitsStillWork(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	key := func(i int) *record.Key { return record.NewKey("Transaction", [32]byte{byte(i)}, "Main") }

	db, err := Open(dir)
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		b := db.Begin(nil, true)
		require.NoError(t, b.Put(key(i), []byte{byte(i)}))
		require.NoError(t, b.Commit())
	}
	require.NoError(t, db.Close())

	db, err = Open(dir)
	require.NoError(t, err)
	defer db.Close()
	for i := 6; i < 9; i++ {
		b := db.Begin(nil, true)
		require.NoError(t, b.Put(key(i), []byte{byte(i)}))
		require.NoErrorf(t, b.Commit(), "commit %d after restart", i)
	}

	// And everything written before and after the restart reads back.
	b := db.Begin(nil, false)
	defer b.Discard()
	for i := 0; i < 9; i++ {
		v, err := b.Get(key(i))
		require.NoErrorf(t, err, "key %d", i)
		require.Equal(t, []byte{byte(i)}, v)
	}
}

func reopen(t *testing.T, dir string, db *Database) *Database {
	t.Helper()
	require.NoError(t, db.Close())
	db, err := Open(dir)
	require.NoError(t, err)
	return db
}

func del(t *testing.T, d *Database, key *record.Key) {
	t.Helper()
	batch := d.Begin(nil, true)
	require.NoError(t, batch.Delete(key))
	require.NoError(t, batch.Commit())
}

// The dyna exception set must survive a restart. A deleted write-once key
// leaves a tombstone in the dynamic layer, which is read first; if the
// process forgets that across a restart, the next write goes to the
// permanent layer and the tombstone shadows it — the record reads as
// deleted while holding a value.
func TestRestart_DeleteBeforeWriteSurvives(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	key := record.NewKey("Message", [32]byte{7}, "Main")
	require.True(t, isWriteOnce(key))

	del(t, db, key)
	db = reopen(t, dir, db)
	defer db.Close()

	put(t, db, key, "value")
	require.Equal(t, "value", get(t, db, key), "written after a restart, must not be shadowed by the pre-restart tombstone")

	db = reopen(t, dir, db)
	require.Equal(t, "value", get(t, db, key), "and still after another restart")
}

// The same for a key the permanent layer refused: its later writes must
// keep going to the dynamic layer after a restart, or a rewrite with the
// ORIGINAL bytes is a permanent-layer no-op while the dynamic layer's
// newer value still shadows it.
func TestRestart_MisrouteSurvives(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	key := record.NewKey("Message", [32]byte{8}, "Main")

	put(t, db, key, "first")
	put(t, db, key, "second") // refused by perm, lands in dyna
	require.Equal(t, "second", get(t, db, key))

	db = reopen(t, dir, db)
	defer db.Close()
	put(t, db, key, "first")
	require.Equal(t, "first", get(t, db, key), "the newest write must win after a restart")
	put(t, db, key, "third")
	require.Equal(t, "third", get(t, db, key))
}

// A commit's own write-through error belongs to that Commit, not the next
// one. Provoked by closing the store underneath the database: the first
// Commit after that must fail, not succeed and poison the one after.
func TestCommit_ReportsItsOwnWriteThroughError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	key := record.NewKey("Message", [32]byte{9}, "Main")
	put(t, db, key, "ok")

	require.NoError(t, db.kv.Close()) // the store fails from here on
	batch := db.Begin(nil, true)
	require.NoError(t, batch.Put(key, []byte("lost")))
	require.Error(t, batch.Commit(), "the commit that could not be written through must be the one that fails")
}
