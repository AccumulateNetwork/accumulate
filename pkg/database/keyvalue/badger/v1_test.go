// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func init() {
	// Suppress Badger logs
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	})))
}

func TestV1WriteLimit(t *testing.T) {
	// Create a badger DB
	raw, err := badger.Open(badger.
		DefaultOptions(t.TempDir()).
		WithMaxTableSize(1 << 20). // 1MB
		WithLogger(slogger{}))
	require.NoError(t, err)
	defer raw.Close()

	// Verify that 2000 entries causes ErrTxnTooBig (when max table size is 1MB)
	txn := raw.NewTransaction(true)
	defer txn.Discard()
	for i := 0; i < 2000; i++ {
		err = txn.Set([]byte(fmt.Sprint(i)), []byte{byte(i)})
		if err == nil {
			continue
		}
	}
	require.ErrorIs(t, err, badger.ErrTxnTooBig)

	// Create a kv db
	db := &Database{badger: raw, ready: true}

	// Verify that the kv db supports writes that exceed badger's limits
	batch := db.Begin(nil, true)
	for i := 0; i < 2000; i++ {
		require.NoError(t, batch.Put(record.NewKey(i), []byte{byte(i)}))
	}
	require.NoError(t, batch.Commit())

	// Verify the values can be read
	batch = db.Begin(nil, false)
	defer batch.Discard()
	for i := 0; i < 2000; i++ {
		_, err = batch.Get(record.NewKey(i))
		require.NoError(t, err)
	}
}

func BenchmarkV1Commit(b *testing.B) {
	kvtest.BenchmarkCommit(b, newOpenerV1(b))
}

func BenchmarkV1ReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, newOpenerV1(b))
}

func TestV1Database(t *testing.T) {
	kvtest.TestDatabase(t, newOpenerV1(t))
}

func TestV1SubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, newOpenerV1(t))
}

func TestV1Prefix(t *testing.T) {
	kvtest.TestPrefix(t, newOpenerV1(t))
}

func TestV1Delete(t *testing.T) {
	kvtest.TestDelete(t, newOpenerV1(t))
}

func newOpenerV1(t testing.TB) kvtest.Opener {
	path := t.TempDir()
	return func() (keyvalue.Beginner, error) {
		return New(path)
	}
}

func TestVersions(t *testing.T) {
	dir := t.TempDir()
	db, err := New(dir)
	require.NoError(t, err)
	defer db.Close()

	// Write a value 100 times
	for i := 0; i < 100; i++ {
		// Write the value
		batch := db.Begin(nil, true)
		defer batch.Discard()
		require.NoError(t, batch.Put(record.NewKey("foo"), []byte{byte(i)}))
		require.NoError(t, batch.Commit())

		// Open and close a batch
		db.Begin(nil, false).Discard()
	}

	// Reopen
	require.NoError(t, db.Close())
	db, err = New(dir)
	require.NoError(t, err)

	tx := db.badger.NewTransaction(false)
	defer tx.Discard()
	opts := badger.DefaultIteratorOptions
	opts.AllVersions = true
	it := tx.NewIterator(opts)
	defer it.Close()

	var count int
	for it.Seek(nil); it.Valid(); it.Next() {
		count++
	}

	// Only one version survives
	require.Equal(t, 1, count)
}
