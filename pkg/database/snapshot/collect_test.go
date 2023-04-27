// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func fillDB(t testing.TB, db *database.Database, N, M int) {
	for i := 0; i < N; i++ {
		batch := db.Begin(true)
		defer batch.Discard()
		for j := 0; j < M; j++ {
			u := protocol.AccountUrl(fmt.Sprint(i*M + j))
			err := batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u})
			require.NoError(t, err)
		}
		require.NoError(t, batch.Commit())
	}
}

func collect(t testing.TB, db *database.Database, w *snapshot.Writer) {
	c, err := w.Open()
	require.NoError(t, err)

	batch := db.Begin(false)
	defer batch.Discard()
	err = batch.ForEachAccount(func(account *database.Account, hash [32]byte) error {
		return c.Collect(account)
	})
	require.NoError(t, err)

	require.NoError(t, c.Close())
}

func BenchmarkCollect(b *testing.B) {
	const M = 1000
	for _, N := range []int{1 /*, 10, 100*/} {
		b.Run(fmt.Sprint(N*M), func(b *testing.B) {
			b.ReportAllocs()
			dir := b.TempDir()
			logger := acctesting.NewTestLogger(b)
			db, err := database.OpenBadger(filepath.Join(dir, "test.db"), logger)
			require.NoError(b, err)
			defer db.Close()
			db.SetObserver(acctesting.NullObserver{})

			// Set up a bunch of accounts
			fillDB(b, db, N, M)

			// Collect
			for i := 0; i < b.N; i++ {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("test.%d.snap", i)))
				require.NoError(b, err)
				defer f.Close()

				w, err := snapshot.Create(f)
				require.NoError(b, err)

				w.WriteHeader(new(snapshot.Header))
				require.NoError(b, err)

				collect(b, db, w)
			}
		})
	}
}

func BenchmarkCollectAndIndex(b *testing.B) {
	const M = 1000
	for _, N := range []int{1, 10, 100} {
		b.Run(fmt.Sprint(N*M), func(b *testing.B) {
			dir := b.TempDir()
			logger := acctesting.NewTestLogger(b)
			db, err := database.OpenBadger(filepath.Join(dir, "test.db"), logger)
			require.NoError(b, err)
			defer db.Close()
			db.SetObserver(acctesting.NullObserver{})

			// Set up a bunch of accounts
			fillDB(b, db, N, M)

			// Collect
			for i := 0; i < b.N; i++ {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("test.%d.snap", i)))
				require.NoError(b, err)
				defer f.Close()

				w, err := snapshot.Create(f)
				require.NoError(b, err)

				w.WriteHeader(new(snapshot.Header))
				require.NoError(b, err)

				collect(b, db, w)
				require.NoError(b, w.WriteIndex())
			}

			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			fmt.Println(ms.HeapAlloc)
		})
	}

}
