// Copyright 2024 The Accumulate Authors
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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

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
		return Open(path)
	}
}

func TestFileLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, WithFileLimit(1<<10))
	require.NoError(t, err)
	defer db.Close()

	batch := db.Begin(nil, true)
	defer batch.Discard()

	const N = 16
	for i := 0; i < N; i++ {
		k := record.NewKey(i)
		v := make([]byte, 128)
		_, _ = rand.Read(v)
		err = batch.Put(k, v)
		require.NoError(t, err, "Put")
	}
	require.NoError(t, batch.Commit())

	var files []string
	ent, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, ent := range ent {
		files = append(files, ent.Name())
	}
	require.Equal(t, []string{
		"0.blocks",
		"1.blocks",
		"2.blocks",
	}, files)
}
