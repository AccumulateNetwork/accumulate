// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"crypto/sha256"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCheckStoreNamesATornChain: a store whose RootChain head names more
// hashes than its Tail chunk holds -- what the kill left on bvn2-val3 in run
// 20261002T145702Z -- is named by chain and record; a whole store passes,
// its early elements in packed history included (#4507).
func TestCheckStoreNamesATornChain(t *testing.T) {
	store, err := bcdb.Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer store.Close()
	store.StatsEvery, store.CompressEvery, store.PackEvery = 0, 5, 10
	require.NoError(t, store.SetMergeLag(20))
	db := database.New(store, nil)
	part := protocol.PartitionUrl("BVN2")
	ledger := part.JoinPath(protocol.Ledger)

	// 83 blocks each appending one root: the open mark set (mark power 8 on
	// a ledger's root chain) holds 83 in eleven chunks.
	for i := 0; i < 83; i++ {
		batch := db.Begin(true)
		h := sha256.Sum256([]byte{byte(i)})
		require.NoError(t, batch.Account(ledger).RootChain().Inner().AddEntry(h[:], false))
		require.NoError(t, batch.Commit())
	}
	require.NoError(t, CheckStore(db, part, []string{protocol.Directory}), "a whole store passes")

	batch := db.Begin(true)
	c := batch.Account(ledger).RootChain().Inner()
	chunk, err := c.Tail(2).Get()
	require.NoError(t, err)
	chunk.Hashes = chunk.Hashes[:4]
	require.NoError(t, c.Tail(2).Put(chunk))
	require.NoError(t, batch.Commit())

	err = CheckStore(db, part, []string{protocol.Directory})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.InvalidRecord))
	require.Contains(t, err.Error(), "RootChain")
	require.Contains(t, err.Error(), "tail chunk 2 holds 4 hashes, the head says 8")
}

// TestATornStoreIsNeverResumedOnto: a state built with a torn finding reports
// nothing to resume from, whatever else would allow the whole-restart exit.
func TestATornStoreIsNeverResumedOnto(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, block: 10}, ExecutedBlock: 7,
		Torn: errors.InvalidRecord.With("store torn: RootChain tail chunk 2 holds 4 hashes, the head says 8")})
	require.NoError(t, err)
	s.peersJoining = true
	_, ok := s.Resumable()
	require.False(t, ok, "a torn store is no block's state")
}
