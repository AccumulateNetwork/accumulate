// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestUpdateAccountAuth_SignatureRequest(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Execute
	env, err := build.Transaction().For(alice, "tokens").
		UpdateAccountAuth().Add(bob, "book").
		SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey).
		Done()
	require.NoError(t, err)
	sim.SubmitTxnSuccessfully(env)

	sig := &messaging.SignatureMessage{Signature: env.Signatures[0]}
	sim.StepUntil(
		Txn(sig.ID()).Succeeds(),
		Txn(sig.ID()).Produced().Succeeds())

	// Ensure the transaction shows up as pending on bob's book
	r := sim.QueryPendingIds(bob.JoinPath("book"), nil)
	require.Len(t, r.Records, 1)
	require.Equal(t, env.Transaction[0].ID().String(), r.Records[0].Value.String())
}

func TestUpdateAccountAuth(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().Add(bob, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Sign with the second authority
	r := sim.QueryTransaction(st.TxID, nil)
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTransaction(r.Message.Transaction).
			Url(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}

func TestUpdateAccountAuth2(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1 * AcmePrecision)})

	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Disable auth
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().Disable(alice, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// An unauthorized signer must not be allowed to enable auth
	sts := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().Enable("alice", "book").
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))
	sim.StepUntil(
		Sig(sts[1].TxID).Succeeds(),
		Sig(sts[1].TxID).AuthoritySignature().Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "acc://bob.acme/book/1 is not authorized to sign transactions for acc://alice.acme/tokens")

	// An unauthorized signer should be able to send tokens
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(68, 0).To(bob, "tokens").
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	require.Equal(t, int64(AcmePrecision-68), GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens")).Balance.Int64())
	require.Equal(t, int64(68), GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens")).Balance.Int64())

	// Enable auth
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().Enable(alice, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// An unauthorized signer should no longer be able to send tokens
	sts = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(68, 0).To(bob, "tokens").
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))
	sim.StepUntil(
		Sig(sts[1].TxID).Succeeds(),
		Sig(sts[1].TxID).AuthoritySignature().Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "acc://bob.acme/book/1 is not authorized to sign transactions for acc://alice.acme/tokens")
	sim.StepUntil(
		Sig(sts[1].TxID).Succeeds(),
		Sig(sts[1].TxID).AuthoritySignature().Capture(&st).Fails())
	require.EqualError(t, st.AsError(), "acc://bob.acme/book/1 is not authorized to sign transactions for acc://alice.acme/tokens")
}
