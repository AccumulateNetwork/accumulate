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
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// A receipt over a slow chain reaches for a mark point written long before
// the window. The Directory's root chain and its per-partition anchor chains
// are such chains, and every Directory anchor carries receipts over them.
// Soak 20260905T032333Z: mark points routed to the permanent layer read as
// absent past the window, the chain rebuilt its state from nothing, and every
// receipt ended at a root no BVN held. The chain must answer the same as an
// unwindowed store, or refuse.
func TestReceiptReachesPastTheWindow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	defer db.Close()
	db.StatsEvery = 0
	require.NoError(t, db.SetMergeLag(20))

	acct := url.MustParse("acc://dn.acme/ledger")
	key := record.NewKey("Account", acct, "RootChain")
	newChain := func(store keyvalue.RecordStore) *merkle.Chain {
		return merkle.NewChain(nil, store, key, 4, merkle.ChainTypeTransaction, "root")
	}

	// The reference: the same entries on an unwindowed store.
	ref := newChain(keyvalue.RecordStore{Store: memory.New(nil).Begin(nil, true)})

	// Sixty commits of five entries: the mark points (every 16 entries) of
	// the first commits are far behind a 20-commit window by the end.
	var hashes [][]byte
	const commits, perCommit = 60, 5
	for i := 0; i < commits; i++ {
		batch := db.Begin(nil, true)
		c := newChain(keyvalue.RecordStore{Store: batch})
		for j := 0; j < perCommit; j++ {
			h := make([]byte, 32)
			h[0], h[1], h[2] = byte(i), byte(j), 7
			hashes = append(hashes, h)
			require.NoError(t, c.AddEntry(h, false))
			require.NoError(t, ref.AddEntry(h, false))
		}
		require.NoError(t, c.Commit(), "flush the chain's records into the batch")
		require.NoError(t, batch.Commit())
	}

	batch := db.Begin(nil, false)
	defer batch.Discard()
	c := newChain(keyvalue.RecordStore{Store: batch})
	head, err := c.Head().Get()
	require.NoError(t, err)
	require.Equal(t, int64(commits*perCommit), head.Count)

	// A state deep in the past, and receipts from early entries to the head:
	// what the Directory builds for an anchor received long ago.
	refState, err := ref.StateAt(37)
	require.NoError(t, err)
	got, err := c.StateAt(37)
	require.NoError(t, err, "the mark point below 37 was written 50 commits ago")
	require.Equal(t, refState.Anchor(), got.Anchor())

	refHead, err := ref.Head().Get()
	require.NoError(t, err)
	for _, from := range []int64{1, 17, 100, 250} {
		r, err := c.Receipt(from, head.Count-1)
		require.NoError(t, err, "receipt from %d", from)
		require.True(t, r.Validate(nil), "receipt from %d validates", from)
		require.Equal(t, refHead.Anchor(), r.Anchor, "receipt from %d ends at the chain's anchor", from)
		require.Equal(t, hashes[from], r.Start)
	}
}
