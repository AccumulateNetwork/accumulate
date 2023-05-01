// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func fillDB(t testing.TB, sim *Sim, N, M int) {
	for i := 0; i < N; i++ {
		accounts := make([]protocol.Account, M)
		for j := 0; j < M; j++ {
			u := protocol.AccountUrl(fmt.Sprint(i*M + j))
			accounts[j] = &protocol.UnknownAccount{Url: u}
		}
		MakeAccount(t, sim.Database("BVN0"), accounts...)
	}

	runtime.GC()
}

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	sim := NewSim(t,
		simulator.BadgerDatabaseFromDirectory(dir, func(err error) { require.NoError(t, err) }),
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// Set up a bunch of accounts
	fillDB(t, sim, 1, 10)

	f, err := os.Create(filepath.Join(dir, "test.snap"))
	require.NoError(t, err)
	defer f.Close()

	err = sim.S.Collect("BVN0", f, nil)
	require.NoError(t, err)

	// Try to read an account out of the snapshot
	_, err = f.Seek(0, io.SeekEnd)
	require.NoError(t, err)
	ss, err := snapshot.Open(f)
	require.NoError(t, err)

	batch := database.NewBatch(t.Name(), fakeChangeSet{ss}, false, nil)
	account, err := batch.Account(protocol.PartitionUrl("BVN0")).Main().Get()
	require.NoError(t, err)
	require.IsType(t, (*protocol.SystemLedger)(nil), account)
}

// func BenchmarkCollect(b *testing.B) {
// 	N := []int{1, 5, 25}
// 	// N := []int{5}
// 	const M = 1000
// 	for _, N := range N {
// 		dir := b.TempDir()
// 		logger := acctesting.NewTestLogger(b)
// 		db, err := database.OpenBadger(filepath.Join(dir, "test.db"), logger)
// 		require.NoError(b, err)
// 		defer db.Close()
// 		db.SetObserver(acctesting.NullObserver{})

// 		// Set up a bunch of accounts
// 		fillDB(b, db, N, M)

// 		var peak uint64
// 		var ms runtime.MemStats
// 		b.Run(fmt.Sprint(N*M), func(b *testing.B) {
// 			// Collect
// 			for i := 0; i < b.N; i++ {
// 				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("test.%d.snap", i)))
// 				require.NoError(b, err)
// 				defer f.Close()

// 				err = db.Collect(f, protocol.DnUrl(), nil)
// 				require.NoError(b, err)

// 				runtime.ReadMemStats(&ms)
// 				if ms.Alloc > peak {
// 					peak = ms.Alloc
// 				}
// 			}
// 		})
// 		fmt.Printf("Peak allocated by %d is %d\n", N*M, peak)
// 	}
// }

type fakeChangeSet struct{ keyvalue.Store }

func (fakeChangeSet) Begin(*record.Key, bool) keyvalue.ChangeSet { panic("shim") }
func (fakeChangeSet) Commit() error                              { panic("shim") }
func (fakeChangeSet) Discard()                                   {}
