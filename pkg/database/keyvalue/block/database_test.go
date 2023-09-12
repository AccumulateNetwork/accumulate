// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"math/rand"
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

	for i := 0; i < N; i++ {
		k := record.NewKey(i)
		v := make([]byte, rand.Intn(5000)+100)
		rand.Read(v)
		err := batch.Put(k, v)
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
