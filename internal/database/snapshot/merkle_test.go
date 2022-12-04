// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestSnapshotPartialHistory(t *testing.T) {
	// Mark power is 8 (mark point is 256), set by a const in
	// internal/database/database.go.

	// See CollectAccount in internal/database/snapshot/records.go for the full
	// history vs truncated history logic.

	// Create a chain with 300 entries
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	foo := protocol.AccountUrl("foo")
	require.NoError(t, batch.Account(foo).Main().Put(&protocol.LiteIdentity{Url: foo}))
	chain, err := batch.Account(foo).MainChain().Get()
	require.NoError(t, err)

	var rh common.RandHash
	for i := 0; i < 300; i++ {
		require.NoError(t, chain.AddEntry(rh.NextList(), false))
	}

	require.NoError(t, batch.Commit())

	// Make a snapshot
	buf := new(ioutil2.Buffer)
	snapWr, err := snapshot.Create(buf, new(snapshot.Header))
	require.NoError(t, err)

	batch = db.Begin(true)
	defer batch.Discard()
	require.NoError(t, snapWr.CollectAccounts(batch, snapshot.CollectOptions{
		PreserveAccountHistory: func(account *database.Account) (bool, error) {
			return false, nil // Do not preserve history
		},
	}))
	batch.Discard()

	require.NoError(t, db.Close())

	// Restore the snapshot to a new database
	store := memory.New(nil)
	db = database.New(store, nil)
	require.NoError(t, snapshot.Restore(db, buf, nil))

	// Verify the account chain
	key := record.Key{"Account", foo, "MainChain"}
	storetx := store.Begin(false)
	defer storetx.Discard()
	c := managed.NewChain(nil, record.KvStore{Store: storetx}, key, 8, managed.ChainTypeTransaction, "main", "main")

	entriesShouldFailOrReturnCorrectNumber(t, c, 0, 300)
	entriesShouldFailOrReturnCorrectNumber(t, c, 100, 300)
	entriesShouldFailOrReturnCorrectNumber(t, c, 0, 256)
	entriesShouldFailOrReturnCorrectNumber(t, c, 256, 300)
	entriesShouldFailOrReturnCorrectNumber(t, c, 200, 400)
}

func TestSnapshotFullHistory(t *testing.T) {
	// Mark power is 8 (mark point is 256), set by a const in
	// internal/database/database.go.

	// Create a chain with 300 entries
	for n := 1; n < 300; n++ {
		db := database.OpenInMemory(nil)
		batch := db.Begin(true)
		defer batch.Discard()

		foo := protocol.AccountUrl("foo")
		require.NoError(t, batch.Account(foo).Main().Put(&protocol.LiteIdentity{Url: foo}))
		chain, err := batch.Account(foo).MainChain().Get()
		require.NoError(t, err)

		var rh common.RandHash
		for i := 0; i < n; i++ {
			require.NoError(t, chain.AddEntry(rh.NextList(), false))
		}

		require.NoError(t, batch.Commit())

		// Make a snapshot
		buf := new(ioutil2.Buffer)
		snapWr, err := snapshot.Create(buf, new(snapshot.Header))
		require.NoError(t, err)

		batch = db.Begin(true)
		defer batch.Discard()
		require.NoError(t, snapWr.CollectAccounts(batch, snapshot.CollectOptions{}))
		batch.Discard()

		require.NoError(t, db.Close())

		// Restore the snapshot to a new database
		store := memory.New(nil)
		db = database.New(store, nil)
		require.NoError(t, snapshot.Restore(db, buf, nil))

		// Verify the account chain
		key := record.Key{"Account", foo, "MainChain"}
		storetx := store.Begin(false)
		defer storetx.Discard()
		c := managed.NewChain(nil, record.KvStore{Store: storetx}, key, 8, managed.ChainTypeTransaction, "main", "main")

		for i := 0; i < n; i++ {
			hash, err := c.Get(int64(i))
			require.NoError(t, err)
			require.Equalf(t, rh.List[i], []byte(hash), "Entry %d", i)
		}
	}
}

func entriesShouldFailOrReturnCorrectNumber(t *testing.T, chain record.Chain, start, end int64) {
	t.Helper()
	hashes, err := chain.GetRange(start, end)
	if err != nil {
		return
	}
	h, err := chain.Head().Get()
	require.NoError(t, err)
	if end > h.Count {
		end = h.Count
	}
	require.Equalf(t, int(end-start), len(hashes), "Range from %d to %d", start, end)
}
