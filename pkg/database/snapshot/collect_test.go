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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func fillDB(t testing.TB, db *database.Database, N, M int) {
	var rh common.RandHash
	for i := 0; i < N; i++ {
		batch := db.Begin(true)
		defer batch.Discard()

		if i == 0 {
			ledger := &protocol.SystemLedger{Url: protocol.DnUrl().JoinPath(protocol.Ledger)}
			err := batch.Account(ledger.GetUrl()).Main().Put(ledger)
			require.NoError(t, err)
		}

		for j := 0; j < M; j++ {
			u := protocol.AccountUrl(fmt.Sprint(i*M + j))
			err := batch.Account(u).Main().Put(&protocol.UnknownAccount{Url: u})
			require.NoError(t, err)

			for z := 0; z < 20; z++ {
				require.NoError(t, batch.Account(u).MainChain().Inner().AddHash(rh.Next(), false))
			}
		}
		require.NoError(t, batch.Commit())
	}

	runtime.GC()
}

func BenchmarkCollect(b *testing.B) {
	// N := []int{1, 5, 25}
	N := []int{5}
	const M = 1000
	for _, N := range N {
		dir := b.TempDir()
		logger := acctesting.NewTestLogger(b)
		db, err := database.OpenBadger(filepath.Join(dir, "test.db"), logger)
		require.NoError(b, err)
		defer db.Close()
		db.SetObserver(acctesting.NullObserver{})

		// Set up a bunch of accounts
		fillDB(b, db, N, M)

		var peak uint64
		var ms runtime.MemStats
		b.Run(fmt.Sprint(N*M), func(b *testing.B) {
			// Collect
			for i := 0; i < b.N; i++ {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("test.%d.snap", i)))
				require.NoError(b, err)
				defer f.Close()

				err = db.Collect(f, protocol.DnUrl(), nil)
				require.NoError(b, err)
			}

			runtime.ReadMemStats(&ms)
			if ms.Alloc > peak {
				peak = ms.Alloc
			}
		})
		fmt.Printf("Peak allocated by %d is %d\n", N*M, peak)
	}
}

// func BenchmarkCollectAndIndex(b *testing.B) {
// 	const M = 1000
// 	for _, N := range []int{1, 10, 100} {
// 		var peak uint64
// 		b.Run(fmt.Sprint(N*M), func(b *testing.B) {
// 			dir := b.TempDir()
// 			logger := acctesting.NewTestLogger(b)
// 			db, err := database.OpenBadger(filepath.Join(dir, "test.db"), logger)
// 			require.NoError(b, err)
// 			defer db.Close()
// 			db.SetObserver(acctesting.NullObserver{})

// 			// Set up a bunch of accounts
// 			fillDB(b, db, N, M)

// 			// Collect
// 			for i := 0; i < b.N; i++ {
// 				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("test.%d.snap", i)))
// 				require.NoError(b, err)
// 				defer f.Close()

// 				w, err := snapshot.Create(f)
// 				require.NoError(b, err)

// 				w.WriteHeader(new(snapshot.Header))
// 				require.NoError(b, err)

// 				collect(b, db, w, &peak)
// 				require.NoError(b, w.WriteIndex())
// 			}

// 			var ms runtime.MemStats
// 			runtime.ReadMemStats(&ms)
// 			fmt.Println(ms.HeapAlloc)
// 		})
// 	}

// }
