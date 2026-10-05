// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestABlockStampsTheAccountsItWritesWithItsIndex — #4518, executor spec
// "Sync" §3a: execution stamps every account it changes with the block it
// executes, and the stamp is outside the hashed state: written alone, it moves
// no leaf and no root.
func TestABlockStampsTheAccountsItWritesWithItsIndex(t *testing.T) {
	db := OpenInMemory(nil)
	u := protocol.DnUrl().JoinPath("stamped")

	// A block that writes the account.
	batch := db.Begin(true)
	require.NoError(t, batch.SettleBlock(7))
	require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	batch = db.Begin(false)
	at, err := batch.Account(u).StateBlock().Get()
	require.NoError(t, err)
	require.Equal(t, uint64(7), at, "the block did not stamp the account it wrote")
	root, err := batch.GetBptRootHash()
	require.NoError(t, err)
	leaf, err := batch.BPT().Get(batch.Account(u).Key())
	require.NoError(t, err)
	batch.Discard()

	// A batch that is no block does not stamp.
	batch = db.Begin(true)
	require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch = db.Begin(false)
	at, err = batch.Account(u).StateBlock().Get()
	require.NoError(t, err)
	require.Equal(t, uint64(7), at, "a batch that is no block changed the stamp")
	batch.Discard()

	// The stamp alone is not state.
	batch = db.Begin(true)
	require.NoError(t, batch.Account(u).StateBlock().Put(9))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch = db.Begin(false)
	defer batch.Discard()
	root2, err := batch.GetBptRootHash()
	require.NoError(t, err)
	leaf2, err := batch.BPT().Get(batch.Account(u).Key())
	require.NoError(t, err)
	require.Equal(t, root, root2, "the stamp moved the root")
	require.Equal(t, leaf, leaf2, "the stamp moved the leaf")
}
