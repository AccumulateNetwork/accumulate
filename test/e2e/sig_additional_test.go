// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestAdditionalAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Initiate
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			AdditionalAuthority(bob, "book").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// Wait for the transaction to appear (it must be pending)
	sim.StepUntil(
		Sig(st[1].TxID).AuthoritySignature().Completes())
	sim.Verify(
		Txn(st[0].TxID).IsPending())

	// Sign with the other authority
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).Completes())
}

func TestUpdateKeyAdditionalAuthority(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	otherKey := acctesting.GenerateKey(alice, "other")
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	var timestamp uint64
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		// Require a second signature from a non-existent authority to ensure
		// UpdateKey skips the normal multisig checks
		page.AddKeySpec(&KeySpec{Delegate: AccountUrl("foo")})
		page.AcceptThreshold = 2
	})

	// Update the key
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			AdditionalAuthority(bob, "book").
			UpdateKey(otherKey, SignatureTypeED25519).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// Wait for the transaction to appear (it must be pending)
	sim.StepUntil(
		Sig(st[1].TxID).AuthoritySignature().Completes())
	sim.Verify(
		Txn(st[0].TxID).IsPending())

	// Sign with the other authority
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Completes(),
		Sig(st[1].TxID).Completes())

	// Verify the key changed
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	_, _, ok := page.EntryByKey(otherKey[32:])
	require.True(t, ok)
}

func TestMissingAdditionalAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	// DO NOT CREATE BOB

	// Initiate, requiring a signature from bob
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			AdditionalAuthority(bob, "book").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// The transaction is rejected due to the missing authority
	var bobSigSt *protocol.TransactionStatus
	sim.StepUntil(
		Txn(st[0].TxID).Fails().WithError(errors.Rejected),
		Sig(st[1].TxID).SignatureRequestTo(bob, "book").AuthoritySignature().Capture(&bobSigSt).Completes())

	bobSig := sim.QueryMessage(bobSigSt.TxID, nil).Message.(*messaging.SignatureMessage).Signature.(*AuthoritySignature)
	require.Equal(t, bobSig.Memo, "acc://bob.acme/book does not exist")
}

func TestMissingCreateAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	// DO NOT CREATE BOB

	// Initiate, requiring a signature from bob
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			WithAuthority(bob, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// The transaction is rejected due to the missing authority
	var bobSigSt *protocol.TransactionStatus
	sim.StepUntil(
		Txn(st[0].TxID).Fails().WithError(errors.Rejected),
		Sig(st[1].TxID).SignatureRequestTo(bob, "book").AuthoritySignature().Capture(&bobSigSt).Completes())

	bobSig := sim.QueryMessage(bobSigSt.TxID, nil).Message.(*messaging.SignatureMessage).Signature.(*AuthoritySignature)
	require.Equal(t, bobSig.Memo, "acc://bob.acme/book does not exist")
}

func TestInvalidCreateAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob)

	// Initiate, requiring a signature from an account that is not a key book
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			WithAuthority(bob).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// The transaction is rejected due to the missing authority
	var bobSigSt *protocol.TransactionStatus
	sim.StepUntil(
		Txn(st[0].TxID).Fails().WithError(errors.Rejected),
		Sig(st[1].TxID).SignatureRequestTo(bob).AuthoritySignature().Capture(&bobSigSt).Completes())

	bobSig := sim.QueryMessage(bobSigSt.TxID, nil).Message.(*messaging.SignatureMessage).Signature.(*AuthoritySignature)
	require.Equal(t, bobSig.Memo, "acc://bob.acme is not an authority")
}
