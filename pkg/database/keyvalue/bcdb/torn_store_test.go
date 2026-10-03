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

// A chain's records are checked against its head: a Tail chunk short of its
// last rewrite and an element never flushed -- what the kill left on
// bvn2-val3 (RootChain Tail(21) 4 of 8; anchors.MainChain.Element.3937 gone)
// and bvn2-val4 (BlockLedgerChain Tail(5) 7 of 8) in run 20261002T145702Z --
// are each named; a whole chain passes, past the window included (#4507).
func TestAChainsRecordsAreCheckedAgainstItsHead(t *testing.T) {
	key := record.NewKey("Account", url.MustParse("acc://bvn-BVN2.acme/ledger"), "RootChain")
	// 83 entries: the open mark set (mark power 4) holds three, in Tail(0).
	db, _ := packedChain(t, filepath.Join(t.TempDir(), "db"), key, 83)
	defer db.Close()
	chain := func(b keyvalue.ChangeSet) *merkle.Chain {
		return merkle.NewChain(nil, keyvalue.RecordStore{Store: b}, key, 4, merkle.ChainTypeTransaction, "root")
	}

	batch := db.Begin(nil, false)
	require.NoError(t, chain(batch).CheckRecords(100), "a whole chain, its early elements in packed history, passes")
	batch.Discard()

	// The tear: the open mark set's first chunk loses its last rewrite.
	batch = db.Begin(nil, true)
	c := chain(batch)
	chunk, err := c.Tail(0).Get()
	require.NoError(t, err)
	chunk.Hashes = chunk.Hashes[:1]
	require.NoError(t, c.Tail(0).Put(chunk))
	require.NoError(t, c.Commit())
	require.NoError(t, batch.Commit())

	batch = db.Begin(nil, false)
	err = chain(batch).CheckRecords(100)
	batch.Discard()
	require.Error(t, err)
	require.Contains(t, err.Error(), "tail chunk 0 holds 1 hashes, the head says 3")
}
