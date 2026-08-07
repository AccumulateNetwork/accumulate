// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

// Tests backing docs/protocol/key-books-and-delegation.md. Each test names the
// claim it verifies so the documentation cannot drift from the executor without
// something here going red.

import (
	"crypto/sha256"
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

// keyHash returns the hash a key page entry stores for a public key.
func keyHash(pub []byte) []byte {
	h := sha256.Sum256(pub)
	return h[:]
}

// TestDelegate_RotatesOwnEntryBelowThreshold verifies the central claim: a
// delegate may change its own entry on its own authority, without reaching the
// page's accept threshold.
//
// The page is 4-of-7 (mirroring staking.acme/book/2 on mainnet) and only one
// delegate signs.
func TestDelegate_RotatesOwnEntryBelowThreshold(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	bobNewKey := acctesting.GenerateKey(bob, "new")

	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		// Six delegate entries plus alice's own key = seven entries, 4 required
		page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		for _, n := range []string{"c", "d", "e", "f", "g"} {
			page.AddKeySpec(&KeySpec{Delegate: AccountUrl(n)})
		}
		page.AcceptThreshold = 4
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	require.Equal(t, 7, len(GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1")).Keys))

	// Bob, one of seven delegates, rotates the key on his own entry — signing
	// only with his own book.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice, "book", "1").
			UpdateKey(bobNewKey, SignatureTypeED25519).
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	// One signature, against a threshold of 4
	sim.StepUntil(Txn(st.TxID).Completes())

	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	bobEntryIdx, _, ok := page.EntryByDelegate(bob.JoinPath("book"))
	require.True(t, ok, "bob's entry should still exist")
	i, _, ok := page.EntryByKey(bobNewKey[32:])
	require.True(t, ok, "UpdateKey should have written the new key")
	require.Equal(t, bobEntryIdx, i, "the new key must be on bob's own entry")
	require.Equal(t, uint64(4), page.AcceptThreshold, "threshold must be untouched")
}

// TestDelegate_UpdateKeyAddsSideKeyPreservingDelegate verifies that UpdateKey
// applied to a delegate-only entry adds a direct signing key while leaving the
// delegation in place (preserveDelegate), and that the resulting side key can
// then sign directly.
func TestDelegate_UpdateKeyAddsSideKeyPreservingDelegate(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	sideKey := acctesting.GenerateKey(bob, "side")

	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		// A delegate-only entry: no key hash
		page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	_, entry, ok := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1")).
		EntryByDelegate(bob.JoinPath("book"))
	require.True(t, ok)
	require.Empty(t, entry.(*KeySpec).PublicKeyHash, "precondition: entry is delegate-only")

	// Bob attaches a side key to his own entry
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice, "book", "1").
			UpdateKey(sideKey, SignatureTypeED25519).
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	idx, _, ok := page.EntryByDelegate(bob.JoinPath("book"))
	require.True(t, ok, "the delegation must be preserved")
	sideIdx, _, ok := page.EntryByKey(sideKey[32:])
	require.True(t, ok, "the entry must also carry the side key")
	require.Equal(t, idx, sideIdx, "the side key must be on the delegated entry itself")

	// UpdateKey deliberately does not bump the page version
	require.Equal(t, uint64(1), page.Version, "UpdateKey must not bump the page version")

	// And the side key can now sign directly for alice's page
	st2 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "data").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(sideKey))
	sim.StepUntil(Txn(st2.TxID).Completes())
}

// TestDelegate_CannotRepointOwnDelegateAlone verifies that changing which book
// an entry delegates to is NOT within a delegate's own authority: it requires
// UpdateKeyPage, which is threshold-governed.
func TestDelegate_CannotRepointOwnDelegateAlone(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeKeyBook(t, sim.DatabaseFor(bob), bob.JoinPath("stakingbook"), bobKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		page.AcceptThreshold = 2 // bob alone cannot reach this
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("stakingbook", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Bob attempts to repoint his own entry at his staking book, signing only
	// with the delegate book that currently holds the entry.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice, "book", "1").
			UpdateKeyPage().
			Update().
			Entry().Owner(bob.JoinPath("book")).FinishEntry().
			To().Owner(bob.JoinPath("stakingbook")).FinishEntry().
			FinishOperation().
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	// It must not execute on bob's signature alone; the page threshold governs
	// UpdateKeyPage, unlike UpdateKey.
	sim.StepUntil(Txn(st.TxID).IsPending())
	sim.StepN(50)

	require.False(t, sim.QueryTransaction(st.TxID, nil).Status.Delivered(),
		"repointing a delegate must not execute below the page threshold")

	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	_, _, ok := page.EntryByDelegate(bob.JoinPath("book"))
	require.True(t, ok, "the original delegation must be unchanged")
	_, _, ok = page.EntryByDelegate(bob.JoinPath("stakingbook"))
	require.False(t, ok, "the new delegation must not have been installed")
}

// TestDelegate_SideKeyDoesNotDoubleCount answers the open question recorded in
// #4079: compareSignatureSetEntries keys the active signature set on
// (KeyIndex, delegation path) rather than key index alone, so in principle a
// single entry holding BOTH a key hash and a delegate might contribute twice
// toward one threshold — once signed directly, once through the delegation.
//
// The page below requires 2 signatures and has exactly one such entry plus one
// unrelated entry. If the double-count were real, signing both ways from that
// one entry would satisfy the threshold and the transaction would execute.
func TestDelegate_SideKeyDoesNotDoubleCount(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	sideKey := acctesting.GenerateKey(bob, "side")

	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		// One entry carrying BOTH a delegate and a key hash
		page.AddKeySpec(&KeySpec{
			Delegate:      bob.JoinPath("book"),
			PublicKeyHash: keyHash(sideKey[32:]),
		})
		page.AcceptThreshold = 2
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Signature 1: directly with the side key (empty delegation path)
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "data").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(sideKey))
	sim.StepUntil(Txn(st.TxID).IsPending())

	// Signature 2: the SAME entry, this time through the delegation
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).Load(sim.Query()).
			Url(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey).
			Delegator(alice, "book", "1"))

	sim.StepN(50)

	// The documented expectation: one entry contributes one signature, so a
	// threshold of 2 is NOT met by signing twice from the same entry.
	require.False(t, sim.QueryTransaction(st.TxID, nil).Status.Delivered(),
		"a single key page entry must not satisfy a threshold of 2 by signing "+
			"both directly and through its delegate")

	// Control: the threshold really is reachable, and the transaction really is
	// otherwise valid — a signature from a DIFFERENT entry completes it. Without
	// this, the assertion above could pass merely because something unrelated
	// was broken.
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).Load(sim.Query()).
			Url(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())
}
