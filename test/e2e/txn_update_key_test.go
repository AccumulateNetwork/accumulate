// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestUpdateKey(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")
	otherKey := acctesting.GenerateKey(alice, "other")

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AddKeySpec(&KeySpec{Delegate: AccountUrl("foo")})
		page.AcceptThreshold = 2
	})

	// Update the key
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().
			For(alice, "book", "1").
			UpdateKey(otherKey, SignatureTypeED25519).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())

	// Verify the key changed
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	_, _, ok := page.EntryByKey(otherKey[32:])
	require.True(t, ok)
}

func TestUpdateKey_HasDelegate(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")
	otherKey := acctesting.GenerateKey(alice, "other")

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.Keys[0].Delegate = AccountUrl("foo")
	})

	// Update the key
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().
			For(alice, "book", "1").
			UpdateKey(otherKey[32:], SignatureTypeED25519).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())

	// Verify the delegate is unchanged
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Len(t, page.Keys, 1)
	require.NotNil(t, page.Keys[0].Delegate)
	require.Equal(t, "foo.acme", page.Keys[0].Delegate.ShortString())
}

func TestUpdateKey_MultiLevel(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice, "main")
	otherKey := acctesting.GenerateKey(alice, "other")
	newKey := acctesting.GenerateKey(alice, "new")

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeKeyBook(t, sim.DatabaseFor(alice), alice.JoinPath("book2"), make([]byte, 32))
	MakeKeyBook(t, sim.DatabaseFor(alice), alice.JoinPath("book3"), otherKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) { page.Keys[0].Delegate = alice.JoinPath("book2") })
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book2", "1"), func(page *KeyPage) { page.Keys[0].Delegate = alice.JoinPath("book3") })
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book3", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })

	// Update the key
	st := sim.Submit(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("book", "1")).
			WithSigner(alice.JoinPath("book3", "1"), 1).
			WithDelegator(alice.JoinPath("book2", "1")).
			WithTimestampVar(&timestamp).
			WithBody(&UpdateKey{NewKeyHash: hash(newKey[32:])}).
			Initiate(SignatureTypeED25519, otherKey).
			Build())
	require.EqualError(t, st[1].AsError(), "cannot updateKey with a delegated signature")
}

func TestUpdateKey_TwoDelegates(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	charlie := url.MustParse("charlie")
	david := url.MustParse("david")
	aliceKey := acctesting.GenerateKey(alice)
	charlieKey := acctesting.GenerateKey(charlie)

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		page.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
		page.AddKeySpec(&KeySpec{Delegate: david.JoinPath("book")})

		// Make sure charlie is in the middle
		require.True(t, page.Keys[0].Delegate.Equal(bob.JoinPath("book")))
		require.True(t, page.Keys[1].Delegate.Equal(charlie.JoinPath("book")))
		require.True(t, page.Keys[2].Delegate.Equal(david.JoinPath("book")))
	})
	CreditCredits(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), 1e9)

	// Update the key
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			UpdateKey(charlieKey, SignatureTypeED25519).
			SignWith(charlie, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(charlieKey))
	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())

	// Verify the key changed
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	_, _, ok := page.EntryByKey(charlieKey[32:])
	require.True(t, ok)
}
