// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// packedChain writes n entries to a chain over n commits on a store that
// packs its finished blocks every ten, closes it so the maintenance in
// flight finishes, and reopens it: the early entries are in packed history.
func packedChain(t *testing.T, dir string, key *record.Key, n int) (*Database, [][]byte) {
	t.Helper()
	db, err := Open(dir)
	require.NoError(t, err)
	db.StatsEvery, db.CompressEvery, db.PackEvery = 0, 5, 10
	require.NoError(t, db.SetMergeLag(20))
	var hashes [][]byte
	for i := 0; i < n; i++ {
		batch := db.Begin(nil, true)
		c := merkle.NewChain(nil, keyvalue.RecordStore{Store: batch}, key, 4, merkle.ChainTypeTransaction, "main")
		h := make([]byte, 32)
		h[0], h[1], h[2] = byte(i), byte(i>>8), 9
		hashes = append(hashes, h)
		require.NoError(t, c.AddEntry(h, false))
		require.NoError(t, c.Commit())
		require.NoError(t, batch.Commit())
	}
	require.NoError(t, db.Close())
	db, err = Open(dir)
	require.NoError(t, err)
	db.StatsEvery = 0
	return db, hashes
}

// A positional permanent record -- an element at an index the chain's head
// accounts for -- is read by a shallow reader however far past the window
// it is. Its absence is never an answer, so the miss is the window and the
// adapter reads history. Run 20261002T145702Z (#4507): two validators'
// chains refilled a Tail chunk from elements past the window through the
// block's batch and failed every block for ever.
func TestAPositionalPermanentRecordIsReadPastTheWindow(t *testing.T) {
	key := record.NewKey("Account", url.MustParse("acc://bvn-BVN2.acme/ledger"), "MainChain")
	db, hashes := packedChain(t, filepath.Join(t.TempDir(), "db"), key, 80)
	defer db.Close()

	batch := db.Begin(nil, false)
	defer batch.Discard()
	c := merkle.NewChain(nil, keyvalue.RecordStore{Store: batch}, key, 4, merkle.ChainTypeTransaction, "main")
	got, err := c.Element(0).Get()
	require.NoError(t, err, "element 0 is in packed history and a shallow reader still gets it")
	require.Equal(t, hashes[0], got)
	require.NotZero(t, db.ShallowMisses()[keyShape(c.Element(0).Key())], "the miss is still counted, so the reader is still named")
	require.NotEmpty(t, db.HistoryReads(), "and so is the history read it became")

	// ElementIndex is not positional: its absence is asked on every unique
	// append, and that probe stays the cheap miss it is.
	_, err = c.ElementIndex(make([]byte, 32)).Get()
	require.Error(t, err)
	require.Len(t, db.HistoryReads(), 1, "no history walk for an element-index probe")
}

// A Tail chunk left short -- a kill lost its last rewrite -- is refilled from
// the chain's elements on the next append, and that works past the window:
// bvn2-val3's RootChain Tail(21) held 4 of 8 hashes and bvn2-val4's
// BlockLedgerChain Tail(5) held 7 of 8 after run 20261002T145702Z's
// restart, and every append failed with "element N is missing" (#4507).
func TestAnAppendRefillsAShortTailChunkPastTheWindow(t *testing.T) {
	key := record.NewKey("Account", url.MustParse("acc://bvn-BVN2.acme/ledger"), "RootChain")
	db, hashes := packedChain(t, filepath.Join(t.TempDir(), "db"), key, 80)
	defer db.Close()

	// The tear: the open mark set's first chunk loses its last rewrites.
	batch := db.Begin(nil, true)
	c := merkle.NewChain(nil, keyvalue.RecordStore{Store: batch}, key, 4, merkle.ChainTypeTransaction, "root")
	head, err := c.Head().Get()
	require.NoError(t, err)
	chunk, err := c.Tail(0).Get()
	require.NoError(t, err)
	require.Greater(t, len(chunk.Hashes), 2)
	chunk.Hashes = chunk.Hashes[:1]
	require.NoError(t, c.Tail(0).Put(chunk))
	require.NoError(t, c.Commit())
	require.NoError(t, batch.Commit())

	// The next append crosses the short chunk's position and refills it
	// from elements written long before the window.
	batch = db.Begin(nil, true)
	defer batch.Discard()
	c = merkle.NewChain(nil, keyvalue.RecordStore{Store: batch}, key, 4, merkle.ChainTypeTransaction, "root")
	next := make([]byte, 32)
	next[0], next[31] = 0xff, 0xff
	require.NoError(t, c.AddEntry(next, false), "the append refills the chunk from the elements")
	r, err := c.Receipt(0, head.Count)
	require.NoError(t, err)
	require.True(t, r.Validate(nil))
	require.Equal(t, hashes[0], r.Start)
}
