// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// hashedView is everything a peer can be told of an account: its leaf, the
// root, and the receipts from its state to the root.
type hashedView struct {
	hash, leaf, root   [32]byte
	state, bpt, anchor []byte
}

func viewHashed(t *testing.T, batch *database.Batch, u *url.URL) hashedView {
	t.Helper()
	var v hashedView
	var err error
	a := batch.Account(u)
	v.hash, err = a.Hash()
	require.NoError(t, err)
	leaf, err := batch.BPT().Get(a.Key())
	require.NoError(t, err)
	copy(v.leaf[:], leaf)
	v.root, err = batch.GetBptRootHash()
	require.NoError(t, err)
	sr, err := a.StateReceipt()
	require.NoError(t, err)
	v.state, err = sr.MarshalBinary()
	require.NoError(t, err)
	br, err := a.BptReceipt()
	require.NoError(t, err)
	v.bpt, err = br.MarshalBinary()
	require.NoError(t, err)
	v.anchor = br.Anchor
	return v
}

// TestTheMaxSyncedEntryIsNotState — #4493, Paul 2026-10-02: "We need to add
// to accounts max synced entry to the account state that is not persisted
// (hashed). Updating accounts will not update this value." Written on an
// executed account, and committed through UpdateBPT so the leaf is
// recomputed, it changes no leaf, no root and no receipt; a block that
// executes against the account leaves it as it was.
func TestTheMaxSyncedEntryIsNotState(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(123, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}
	send(1) // The account has executed and holds a main chain
	sim.StepN(5)

	tokens := alice.JoinPath("tokens")
	db := sim.DatabaseFor(alice)
	var before hashedView
	View(t, db, func(batch *database.Batch) { before = viewHashed(t, batch, tokens) })

	mark := &database.SyncedEntries{Synced: 7, Top: 11, Low: 3}
	Update(t, db, func(batch *database.Batch) {
		require.NoError(t, batch.Account(tokens).SyncedEntries("main").Put(mark))
		require.NoError(t, batch.Account(tokens).SyncedEntries("signature").Put(mark))
		// The leaf is recomputed: the account is dirty
		require.NoError(t, batch.UpdateBPT())
	})

	var after hashedView
	View(t, db, func(batch *database.Batch) { after = viewHashed(t, batch, tokens) })
	require.Equal(t, before.hash, after.hash, "the account's hash")
	require.Equal(t, before.leaf, after.leaf, "the account's leaf")
	require.Equal(t, before.root, after.root, "the BPT root")
	require.Equal(t, before.state, after.state, "the state receipt")
	require.Equal(t, before.bpt, after.bpt, "the BPT receipt")

	// A block that executes against the account does not touch it
	send(2)
	sim.StepN(5)
	View(t, db, func(batch *database.Batch) {
		got, err := batch.Account(tokens).SyncedEntries("main").Get()
		require.NoError(t, err)
		require.True(t, mark.Equal(got), "execution updated the max synced entry")
		require.NotEqual(t, before.root, viewHashed(t, batch, tokens).root, "the block did not change the account")
	})
}
