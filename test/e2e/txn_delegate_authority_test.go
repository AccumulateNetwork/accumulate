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
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
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

// TestDelegate_SideKeyVsDelegatePaysDifferentCredits verifies who pays for a
// signature when an entry carries both a key hash and a delegate.
//
// The fee is charged to ctx.signer, which is the signer named on the signature
// itself (SignatureContext.getSigner). For a delegated signature that is the
// delegate's own page; for a direct signature it is the page holding the entry.
// So the two signing paths bill different accounts.
func TestDelegate_SideKeyVsDelegatePaysDifferentCredits(t *testing.T) {
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
		// One entry carrying BOTH a delegate and a side key
		page.AddKeySpec(&KeySpec{
			Delegate:      bob.JoinPath("book"),
			PublicKeyHash: keyHash(sideKey[32:]),
		})
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	credits := func(u *url.URL) uint64 {
		return GetAccount[*KeyPage](t, sim.DatabaseFor(u.Identity()), u).CreditBalance
	}
	aliceBefore, bobBefore := credits(alice.JoinPath("book", "1")), credits(bob.JoinPath("book", "1"))

	// Path 1: the side key signs DIRECTLY on alice's page
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "data1").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(sideKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	aliceAfter1, bobAfter1 := credits(alice.JoinPath("book", "1")), credits(bob.JoinPath("book", "1"))
	require.Less(t, aliceAfter1, aliceBefore,
		"a direct signature must be paid by the page holding the entry")
	require.Equal(t, bobBefore, bobAfter1,
		"the delegate's page must not pay for a direct signature")

	// Path 2: the same entry signs THROUGH the delegation
	st2 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "data2").
			SignWith(bob, "book", "1").Delegator(alice, "book", "1").
			Version(1).Timestamp(&timestamp).PrivateKey(bobKey))
	sim.StepUntil(Txn(st2.TxID).Completes())

	aliceAfter2, bobAfter2 := credits(alice.JoinPath("book", "1")), credits(bob.JoinPath("book", "1"))
	require.Less(t, bobAfter2, bobAfter1,
		"a delegated signature must be paid by the delegate's own page")
	require.Equal(t, aliceAfter1, aliceAfter2,
		"the delegating page must not pay for a delegated signature")
}

// TestDelegate_InertHomePageButUsableSidecar tests a containment construction:
// park the sidecar key on its own page together with a bogus key nobody holds,
// set that page's threshold to 2, and the page can never reach its threshold —
// so the key cannot sign for anything its own book authorizes. The same key is
// also attached as a side key on a delegate entry in ANOTHER book's page, where
// it is matched by key hash and its home page's threshold is irrelevant.
//
// Claim under test: the key is inert through its own book, yet usable as the
// sidecar.
func TestDelegate_InertHomePageButUsableSidecar(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	sidecarKey := acctesting.GenerateKey(alice, "sidecar")
	bogusKey := acctesting.GenerateKey(alice, "bogus") // private half never used

	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// alice/book/2: the sidecar key plus a bogus key, threshold 2. Nobody can
	// ever produce two signatures here.
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"),
		sidecarKey[32:], bogusKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "2"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AcceptThreshold = 2
	})

	// bob/book/1 carries a delegate entry for alice's book, with the sidecar
	// key attached to that entry as a side key.
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AddKeySpec(&KeySpec{
			Delegate:      alice.JoinPath("book"),
			PublicKeyHash: keyHash(sidecarKey[32:]),
		})
	})

	// Claim 1: the sidecar key cannot sign through its own page. alice/book/2
	// needs 2 signatures and only one key is usable.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "viaOwnPage").
			SignWith(alice, "book", "2").Version(1).Timestamp(&timestamp).PrivateKey(sidecarKey))
	sim.StepUntil(Txn(st.TxID).IsPending())
	sim.StepN(50)
	require.False(t, sim.QueryTransaction(st.TxID, nil).Status.Delivered(),
		"the sidecar key must not be able to sign through its own page")

	// Claim 2: the same key CAN sign where its hash sits on another page's
	// entry. Here the signer is bob/book/1, whose threshold is 1.
	st2 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(bob).
			CreateDataAccount(bob, "viaSidecar").
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(sidecarKey))
	sim.StepUntil(Txn(st2.TxID).Completes())

	// Control: alice's own page 1 is unaffected — the book still works.
	st3 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "viaPage1").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st3.TxID).Completes())
}

// TestDelegate_DeadKeyHashViaTransactions exercises the sidecar containment
// construction through real transactions rather than database setup, using an
// obviously-bogus key hash in the canonical AC1 display encoding.
//
// The dead entry is a hash with no known preimage, so no private key can ever
// satisfy it — unlike a generated-then-discarded keypair, which is only as dead
// as one's confidence that the private half was destroyed. Encoding it as an
// AC1 address means it renders like any other key in tooling, and the DEADBEEF
// payload makes it self-documenting to anyone auditing the page.
func TestDelegate_DeadKeyHashViaTransactions(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	sidecarKey := acctesting.GenerateKey(alice, "sidecar")

	// An obviously bogus key hash. Round-trips through the AC1 encoding, so it
	// can be handed to tooling as a normal address.
	deadHash, err := hex.DecodeString(
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, err)
	deadAddr := address.FormatAC1(deadHash)
	parsed, err := address.Parse(deadAddr)
	require.NoError(t, err, "the dead address must parse")
	gotHash, ok := parsed.GetPublicKeyHash()
	require.True(t, ok)
	require.Equal(t, deadHash, gotHash, "the dead address must round-trip to the same hash")

	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// alice/book/2 starts with only the sidecar key
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"), sidecarKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "2"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// bob carries a delegate entry for alice's book with the sidecar key
	// attached as a side key
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		page.AddKeySpec(&KeySpec{
			Delegate:      alice.JoinPath("book"),
			PublicKeyHash: keyHash(sidecarKey[32:]),
		})
	})

	// Step 1: add the dead key hash to alice/book/2, signed by the
	// higher-priority page 1
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice, "book", "2").
			UpdateKeyPage().
			Add().Entry().Hash(deadHash).FinishEntry().FinishOperation().
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Completes())

	// Step 2: raise the threshold to 2, which the page can now never reach
	st2 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice, "book", "2").
			UpdateKeyPage().
			SetThreshold(2).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st2.TxID).Completes())

	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "2"))
	require.Equal(t, 2, len(page.Keys), "page should hold the sidecar key and the dead entry")
	require.Equal(t, uint64(2), page.AcceptThreshold)
	_, _, ok = page.EntryByKeyHash(deadHash)
	require.True(t, ok, "the dead entry must be present")
	// Unlike UpdateKey, each UpdateKeyPage bumps the page version: 1 -> 3 here
	require.Equal(t, uint64(3), page.Version,
		"UpdateKeyPage bumps the page version, unlike UpdateKey")

	// The sidecar key cannot sign through its own page
	st3 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(alice).
			CreateDataAccount(alice, "viaOwnPage").
			SignWith(alice, "book", "2").Version(3).Timestamp(&timestamp).PrivateKey(sidecarKey))
	sim.StepUntil(Txn(st3.TxID).IsPending())
	sim.StepN(50)
	require.False(t, sim.QueryTransaction(st3.TxID, nil).Status.Delivered(),
		"the sidecar key must not be able to sign through its own page")

	// ...but still signs as the sidecar on bob's page
	st4 := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().
			For(bob).
			CreateDataAccount(bob, "viaSidecar").
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(sidecarKey))
	sim.StepUntil(Txn(st4.TxID).Completes())
}
