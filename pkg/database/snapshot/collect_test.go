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
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestCollect(t *testing.T) {
	// Setup
	dir := t.TempDir()
	logger := acctesting.NewTestLogger(t)
	db, err := coredb.OpenBadger(filepath.Join(dir, "test.db"), logger)
	require.NoError(t, err)
	defer db.Close()
	db.SetObserver(acctesting.NullObserver{})
	fillDB(t, db, 1, 10)

	// Collect a snapshot
	f, err := os.Create(filepath.Join(dir, "test.snap"))
	require.NoError(t, err)
	defer f.Close()
	collect(t, db, f, protocol.DnUrl())

	// Try to read an account out of the snapshot
	rd, err := snapshot.Open(f)
	require.NoError(t, err)
	ss, err := rd.AsStore()
	require.NoError(t, err)

	batch := coredb.NewBatch(t.Name(), fakeChangeSet{ss}, false, nil)
	account, err := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Main().Get()
	require.NoError(t, err)
	require.IsType(t, (*protocol.SystemLedger)(nil), account)
}

func BenchmarkCollect(b *testing.B) {
	N := []int{1, 5, 25}
	const M = 1000
	for _, N := range N {
		dir := b.TempDir()
		logger := acctesting.NewTestLogger(b)
		db, err := coredb.OpenBadger(filepath.Join(dir, "test.db"), logger)
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

				runtime.ReadMemStats(&ms)
				if ms.Alloc > peak {
					peak = ms.Alloc
				}
			}
		})
		runtime.GC()
		fmt.Printf("Peak allocated by %d is %d\n", N*M, peak)
	}
}

func fillDB(t testing.TB, db *coredb.Database, N, M int) {
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

func collect(t testing.TB, db *coredb.Database, file io.WriteSeeker, partition *url.URL) {
	// Start the snapshot
	w, err := snapshot.Create(file)
	require.NoError(t, err)
	// Load the ledger
	batch := db.Begin(false)
	defer batch.Discard()

	var ledger *protocol.SystemLedger
	err = batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	require.NoError(t, err)

	// Load the BPT root hash
	rootHash, err := batch.BPT().GetRootHash()
	require.NoError(t, err)

	// Write the header
	err = w.WriteHeader(&snapshot.Header{
		RootHash:     rootHash,
		SystemLedger: ledger,
	})
	require.NoError(t, err)

	// Open a records section
	records, err := w.OpenRecords()
	require.NoError(t, err)

	// Iterate over the BPT and collect accounts
	it := batch.IterateAccounts()
	for {
		account, ok := it.Next()
		if !ok {
			break
		}

		// Collect the account's records
		err = records.Collect(account, database.WalkOptions{
			IgnoreIndices: true,
		})
		require.NoError(t, err)
	}
	require.NoError(t, it.Err())

	err = records.Close()
	require.NoError(t, err)

	err = w.WriteIndex()
	require.NoError(t, err)
}

type fakeChangeSet struct{ keyvalue.Store }

func (fakeChangeSet) Begin(*record.Key, bool) keyvalue.ChangeSet { panic("shim") }
func (fakeChangeSet) Commit() error                              { panic("shim") }
func (fakeChangeSet) Discard()                                   {}
