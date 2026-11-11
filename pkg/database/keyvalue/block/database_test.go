// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func TestFoo(t *testing.T) {
	t.Skip("Manual")
	db, err := Open("../../../../.nodes/devnet/bvn1-1/bvnn/data/accumulate.db")
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

func BenchmarkCommit(b *testing.B) {
	kvtest.BenchmarkCommit(b, newOpener(b))
}

func BenchmarkOpen(b *testing.B) {
	kvtest.BenchmarkOpen(b, newOpener(b))
}

func BenchmarkReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, newOpener(b))
}

func TestCommitTime(t *testing.T) {
	t.Skip("Manual")

	const N = 1e6

	db, err := Open(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	batch := db.Begin(nil, true)
	defer batch.Discard()

	max := big.NewInt(5000)
	for i := 0; i < N; i++ {
		n, err := rand.Int(rand.Reader, max)
		require.NoError(t, err)
		k := record.NewKey(i)
		v := make([]byte, n.Int64()+100)
		_, _ = rand.Read(v)
		err = batch.Put(k, v)
		require.NoError(t, err, "Put")
	}

	// Commit
	start := time.Now()
	require.NoError(t, batch.Commit())
	fmt.Println(time.Since(start))
}

func TestDatabase(t *testing.T) {
	kvtest.TestDatabase(t, newOpener(t))
}

func TestIsolation(t *testing.T) {
	kvtest.TestIsolation(t, newOpener(t))
}

func TestSubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, newOpener(t))
}

func TestPrefix(t *testing.T) {
	kvtest.TestPrefix(t, newOpener(t))
}

func TestDelete(t *testing.T) {
	kvtest.TestDelete(t, newOpener(t))
}

func newOpener(t testing.TB) kvtest.Opener {
	path := t.TempDir()
	return func() (keyvalue.Beginner, error) {
		return Open(filepath.Join(path, "test.db"))
	}
}

func TestFileLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, WithFileLimit(2<<10))
	require.NoError(t, err)
	defer db.Close()

	batch := db.Begin(nil, true)
	defer batch.Discard()

	const N = 16
	var keys []string
	for i := 0; i < N; i++ {
		k := record.NewKey(i)
		keys = append(keys, k.String())
		v := make([]byte, 128)
		_, _ = rand.Read(v)
		err = batch.Put(k, v)
		require.NoError(t, err, "Put")
	}
	require.NoError(t, batch.Commit())
	require.NoError(t, db.Close())

	var files []string
	ent, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, ent := range ent {
		if strings.HasSuffix(ent.Name(), dotBlocks) {
			files = append(files, ent.Name())
		}
	}
	require.ElementsMatch(t, []string{
		"1.blocks",
		"1-1.blocks",
		"1-2.blocks",
	}, files)

	db, err = Open(dir, WithFileLimit(1<<10))
	require.NoError(t, err)
	defer db.Close()

	batch = db.Begin(nil, true)
	defer batch.Discard()

	var keys2 []string
	require.NoError(t, batch.ForEach(func(key *record.Key, _ []byte) error {
		keys2 = append(keys2, key.String())
		return nil
	}))
	require.ElementsMatch(t, keys, keys2)
}

func TestReadOnly(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	kvtest.TestReadOnly(t,
		func() (keyvalue.Beginner, error) { return Open(filepath.Join(dir, "db")) },
		func() (keyvalue.Beginner, error) { return Open(filepath.Join(dir, "db"), ReadOnly()) },
		func() error {
			db, err := Open(missing, ReadOnly())
			if err == nil {
				_ = db.Close()
			}
			_, serr := os.Stat(missing)
			require.True(t, os.IsNotExist(serr), "read-only open created the directory")
			return err
		})
}

func commitOne(t *testing.T, dir string) {
	db, err := Open(dir)
	require.NoError(t, err)
	batch := db.Begin(nil, true)
	require.NoError(t, batch.Put(record.NewKey("a"), []byte("b")))
	require.NoError(t, batch.Commit())
	batch.Discard()
	require.NoError(t, db.Close())
}

// A read-only open leaves every file as it was, and works on files that cannot
// be written.
func TestReadOnlyDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	commitOne(t, dir)

	snapshot := func() map[string]string {
		m := map[string]string{}
		require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			m[p] = string(b)
			return err
		}))
		return m
	}
	before := snapshot()
	require.NotEmpty(t, before)

	// Make everything read-only on disk
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			err = os.Chmod(p, 0400)
		}
		return err
	}))

	ro, err := Open(dir, ReadOnly())
	require.NoError(t, err)
	b := ro.Begin(nil, false)
	v, err := b.Get(record.NewKey("a"))
	require.NoError(t, err)
	require.Equal(t, "b", string(v))
	b.Discard()
	w := ro.Begin(nil, true)
	require.NoError(t, w.Put(record.NewKey("c"), []byte("d")))
	require.ErrorIs(t, w.Commit(), ErrReadOnly)
	w.Discard()
	require.NoError(t, ro.Close())
	require.Equal(t, before, snapshot())
}

// Indexing writes files, so an unindexed database is refused.
func TestReadOnlyNotIndexed(t *testing.T) {
	dir := t.TempDir()
	commitOne(t, dir)

	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), indexFilePrefix) {
			err = os.Remove(p)
		}
		return err
	}))

	_, err := Open(dir, ReadOnly())
	require.ErrorIs(t, err, ErrNotIndexed)

	// Read-write rebuilds the index, after which read-only works
	db, err := Open(dir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	db, err = Open(dir, ReadOnly())
	require.NoError(t, err)
	require.NoError(t, db.Close())
}
