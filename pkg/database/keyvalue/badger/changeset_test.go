// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"fmt"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
)

func init() {
	// Suppress Badger logs
	slog.SetDefault(slog.New(slog.HandlerOptions{
		Level: slog.LevelError,
	}.NewTextHandler(os.Stderr)))
}

func TestWriteLimit(t *testing.T) {
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

func BenchmarkCommit(b *testing.B) {
	kvtest.BenchmarkCommit(b, newOpener(b))
}

func BenchmarkReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, newOpener(b))
}

func TestDatabase(t *testing.T) {
	kvtest.TestDatabase(t, newOpener(t))
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
		return New(path)
	}
}
