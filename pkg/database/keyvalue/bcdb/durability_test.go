// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Durability is the commit (database invariant 5), whoever else is reading.
// The reader is never closed and neither is the first instance: a second
// instance opened on the same directory stands in for the process that
// comes back after a crash. Before D5's fix the commit sat in memory
// behind the reader and the second instance saw nothing.
func TestCommitIsDurableWhileAReaderIsOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bvnn", "data", "accumulate.db")
	a, err := Open(dir)
	require.NoError(t, err)

	reader := a.Begin(nil, false) // pins version 0, never released
	_ = reader
	putMain(t, a, "alpha", "1")
	putMain(t, a, "beta", "2")

	b, err := Open(dir)
	require.NoError(t, err)
	defer b.Close()
	r := b.Begin(nil, false)
	defer r.Discard()
	v, err := r.Get(record.NewKey("Account", "alpha", "Main"))
	require.NoError(t, err, "a commit made while a reader was open must be on disk")
	require.Equal(t, []byte("1"), v)
	v, err = r.Get(record.NewKey("Account", "beta", "Main"))
	require.NoError(t, err)
	require.Equal(t, []byte("2"), v)
}

// Isolation is by pre-image, not by delaying the write: a reader begun
// before a run of commits keeps seeing the state at its version -- old
// values for rewritten keys, nothing for keys created later, and still the
// value for keys deleted later -- for Get and for ForEach alike.
func TestReaderStaysAtItsVersionWhileCommitsLand(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer db.Close()

	putMain(t, db, "rewritten", "old")
	putMain(t, db, "deleted", "gone-later")

	reader := db.Begin(nil, false)
	defer reader.Discard()

	putMain(t, db, "rewritten", "new")
	putMain(t, db, "created", "later")
	delMain(t, db, "deleted")
	putMain(t, db, "rewritten", "newer") // a second rewrite: the EARLIEST pre-image must win

	get := func(k string) (string, error) {
		v, err := reader.Get(record.NewKey("Account", k, "Main"))
		return string(v), err
	}
	v, err := get("rewritten")
	require.NoError(t, err)
	require.Equal(t, "old", v, "a rewritten key reads as it was at the reader's version")
	_, err = get("created")
	require.ErrorAs(t, err, new(*database.NotFoundError), "a key created after the reader began does not exist for it")
	v, err = get("deleted")
	require.NoError(t, err)
	require.Equal(t, "gone-later", v, "a key deleted after the reader began still exists for it")

	seen := map[string]string{}
	require.NoError(t, reader.ForEach(func(k *record.Key, v []byte) error {
		seen[k.String()] = string(v)
		return nil
	}))
	byHash := func(k string) string { return record.KeyFromHash(record.NewKey("Account", k, "Main").Hash()).String() }
	require.Equal(t, "old", seen[byHash("rewritten")])
	require.Equal(t, "gone-later", seen[byHash("deleted")])
	_, created := seen[byHash("created")]
	require.False(t, created, "ForEach at the reader's version must not yield a key created later")

	// A new reader sees the present.
	now := db.Begin(nil, false)
	defer now.Discard()
	v2, err := now.Get(record.NewKey("Account", "rewritten", "Main"))
	require.NoError(t, err)
	require.Equal(t, []byte("newer"), v2)
	_, err = now.Get(record.NewKey("Account", "deleted", "Main"))
	require.ErrorAs(t, err, new(*database.NotFoundError))
}

// The overlays a reader needs are dropped when it closes, and a commit with
// no reader open records nothing at all.
func TestOverlaysFollowTheReaders(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer db.Close()

	putMain(t, db, "k", "0") // no reader: no overlay
	require.Empty(t, db.undoVersions)

	r1 := db.Begin(nil, false)
	putMain(t, db, "k", "1")
	r2 := db.Begin(nil, false)
	putMain(t, db, "k", "2")
	require.Len(t, db.undoVersions, 2, "one overlay per commit made while a reader predates it")

	r1.Discard()
	require.Len(t, db.undoVersions, 1, "r2 is at version 2 and still needs commit 3's overlay")
	r2.Discard()
	require.Empty(t, db.undoVersions, "no reader, nothing to remember")
	require.Equal(t, 0.0, testutil.ToFloat64(stagedCommitsGauge.WithLabelValues("db")))
}

// A view is tagged with the function that begun it, so a soak can say who is
// holding an old version rather than that someone is.
func TestViewOpenerIsNamed(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer db.Close()

	r := beginFromAHelperWithADistinctName(db)
	defer r.Discard()
	v, ok := db.oldestView()
	require.True(t, ok)
	require.Contains(t, db.viewOpener[v], "beginFromAHelperWithADistinctName")
}

func beginFromAHelperWithADistinctName(db *Database) keyvalue.ChangeSet {
	return db.Begin(nil, false)
}

func putMain(t *testing.T, db *Database, k, v string) {
	t.Helper()
	b := db.Begin(nil, true)
	require.NoError(t, b.Put(record.NewKey("Account", k, "Main"), []byte(v)))
	require.NoError(t, b.Commit())
}

func delMain(t *testing.T, db *Database, k string) {
	t.Helper()
	b := db.Begin(nil, true)
	require.NoError(t, b.Delete(record.NewKey("Account", k, "Main")))
	require.NoError(t, b.Commit())
}
