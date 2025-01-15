// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/ed25519"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestAdvancedSigning(t *testing.T) {
	type Vote struct {
		Vote  VoteType
		Count int
	}
	type Case struct {
		Keys     uint64
		Accept   uint64
		Reject   uint64
		Response uint64
		Votes    []Vote
		Result   errors.Status
	}
	cases := []Case{
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 6}}, errors.Delivered},                      // Accepts
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 1}, {VoteTypeReject, 6}}, errors.Rejected},  // Rejects - implicit reject threshold
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 1}, {VoteTypeAbstain, 7}}, errors.Rejected}, // Abstains
		{8, 4, 0, 0, []Vote{{VoteTypeAccept, 4}, {VoteTypeReject, 4}}, errors.Delivered}, // Accepts and rejects - FCFS
		{8, 6, 1, 0, []Vote{{VoteTypeAccept, 5}, {VoteTypeReject, 1}}, errors.Rejected},  // Rejects - explicit reject threshold
		{8, 4, 0, 6, []Vote{{VoteTypeAccept, 4}}, errors.Pending},                        // Response threshold - insufficient
		{8, 4, 0, 6, []Vote{{VoteTypeAccept, 4}, {VoteTypeReject, 2}}, errors.Delivered}, // Response threshold
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 3}, {VoteTypeReject, 3}}, errors.Rejected},  // Deadlock
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			alice := AccountUrl("alice")
			alicePriv := make([]ed25519.PrivateKey, c.Keys)
			alicePub := make([][]byte, c.Keys)
			for i := range alicePriv {
				alicePriv[i] = acctesting.GenerateKey(alice, i)
				alicePub[i] = alicePriv[i][32:]
			}

			// Initialize
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.Genesis(GenesisTime),
			)

			MakeIdentity(t, sim.DatabaseFor(alice), alice, alicePub...)
			CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
			MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
			CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

			// Set thresholds
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "book", "1").
					UpdateKeyPage().
					SetThreshold(c.Accept).
					SetRejectThreshold(c.Reject).
					SetResponseThreshold(c.Response).
					SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(alicePriv[0]))

			sim.StepUntil(
				Txn(st.TxID).Completes())

			// Build the transaction
			txn, err := build.Transaction().For(alice, "tokens").BurnTokens(1, 0).Done()
			require.NoError(t, err)

			// Build the signatures
			var i int
			var sigs []Signature
			for _, v := range c.Votes {
				for j := 0; j < v.Count; j++ {
					env, err := build.SignatureForTransaction(txn).Url(alice, "book", "1").Timestamp(2).Version(2).
						Vote(v.Vote).PrivateKey(alicePriv[i]).
						Done()
					require.NoError(t, err)
					sigs = append(sigs, env.Signatures...)
					i++
				}
			}

			// Submit the signature
			st = sim.SubmitTxnSuccessfully(&messaging.Envelope{Transaction: []*Transaction{txn}, Signatures: sigs})

			// Wait for the signatures to propagate
			sim.StepN(50)

			// Verify
			require.ErrorIs(t, sim.QueryTransaction(st.TxID, nil).Status, c.Result)
		})
	}
}

// TestBlockHold verifies that block holds work as advertized.
func TestBlockHold(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("bump")})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Wait for non-zero DN height
	sim.StepUntil(
		DnHeight(1).OnPartition("BVN0"))

	// Initiate two transactions with block thresholds
	st1 := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			HoldUntil(HoldUntilOptions{MinorBlock: 100}).
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey))
	st2 := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			HoldUntil(HoldUntilOptions{MinorBlock: 200}).
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey))

	// It must be pending
	sim.StepUntil(
		Txn(st1[0].TxID).IsPending(),
		Sig(st1[1].TxID).Succeeds(),
		Sig(st1[1].TxID).CreditPayment().Completes(),
		Txn(st2[0].TxID).IsPending(),
		Sig(st2[1].TxID).Succeeds(),
		Sig(st2[1].TxID).CreditPayment().Completes())

	// Verify the state before
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		part, err := sim.Router().RouteAccount(alice)
		require.NoError(t, err)

		record := batch.Account(PartitionUrl(part).JoinPath(Ledger)).Events().Minor()
		blocks, err := record.Blocks().Get()
		require.NoError(t, err)
		assert.Equal(t, []uint64{100, 200}, blocks)

		sigs, err := record.Votes(100).Get()
		require.NoError(t, err)
		assert.Len(t, sigs, 1)

		sigs, err = record.Votes(200).Get()
		require.NoError(t, err)
		assert.Len(t, sigs, 1)
	})

	// Step until the threshold
	sim.StepN(100 - int(sim.S.BlockIndex(Directory)))

	// Bump
	sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "bump").
			WriteData().DoubleHash().
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey))

	// The first transaction should succeed now
	sim.StepUntil(
		Txn(st1[0].TxID).Completes(),
		Sig(st1[1].TxID).Completes())

	// But the second should remain pending
	sim.StepUntil(
		Txn(st2[0].TxID).IsPending(),
		Sig(st2[1].TxID).Succeeds(),
		Sig(st2[1].TxID).CreditPayment().Completes())

	// Verify the state after
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		part, err := sim.Router().RouteAccount(alice)
		require.NoError(t, err)

		record := batch.Account(PartitionUrl(part).JoinPath(Ledger)).Events().Minor()
		blocks, err := record.Blocks().Get()
		require.NoError(t, err)
		assert.Equal(t, []uint64{200}, blocks)

		sigs, err := record.Votes(100).Get()
		require.NoError(t, err)
		assert.Empty(t, sigs)

		sigs, err = record.Votes(200).Get()
		require.NoError(t, err)
		assert.Len(t, sigs, 1)
	})
}

// TestBlockHoldPriority verifies that, while a block hold is in place, a higher
// priority page can override the pending vote of a lower priority page.
func TestBlockHoldPriority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey1 := acctesting.GenerateKey(alice, 1)
	aliceKey2 := acctesting.GenerateKey(alice, 2)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:])
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"), aliceKey2[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "2"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("bump")})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Wait for non-zero DN height
	sim.StepUntil(
		DnHeight(1).OnPartition("BVN0"))

	// Initiate with page 1 with a hold
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			HoldUntil(HoldUntilOptions{MinorBlock: 100}).
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Reject with page 2
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Reject().
			Url(alice, "book", "2").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey2))

	// Step until the threshold
	sim.StepN(100 - int(sim.S.BlockIndex(Directory)))

	// Bump
	sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "bump").
			WriteData().DoubleHash().
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// The transaction succeeds (page 1 wins)
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Initiate with page 2 with a hold
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			HoldUntil(HoldUntilOptions{MinorBlock: 200}).
			BurnTokens(1, 0).
			SignWith(alice, "book", "2").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey2))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Reject with page 1
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Reject().
			Url(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// Step until the threshold
	sim.StepN(200 - int(sim.S.BlockIndex(Directory)))

	// Bump
	sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "bump").
			WriteData().DoubleHash().
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// The transaction is rejected (page 1 wins)
	sim.StepUntil(
		Txn(st.TxID).Fails().
			WithError(errors.Rejected))
}

// TestBlockHoldPriority2 verifies that, while a block hold is in place, a
// partial vote (one that does not reach the threshold) from a higher priority
// page will not confuse the prioritization of votes from lower priority pages.
//
//  1. Page 3 (M=1) initiates (with accept)
//  2. Page 1 (M=3) abstains
//  3. Page 2 (M=1) rejects
//
// In this case, page 1's vote is ignored as it does not reach the threshold so
// page 2's vote wins. There was a bug in the original implementation that
// caused page 3's vote to be recorded as priority 1.
func TestBlockHoldPriority2(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey1 := acctesting.GenerateKey(alice, 1)
	aliceKey2 := acctesting.GenerateKey(alice, 2)
	aliceKey3 := acctesting.GenerateKey(alice, 3)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:], aliceKey2[32:], aliceKey3[32:])
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"), aliceKey1[32:])
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"), aliceKey1[32:])

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
		p.AcceptThreshold = 2
	})
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "2"), 1e9)
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "3"), 1e9)

	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("bump")})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Wait for non-zero DN height
	sim.StepUntil(
		DnHeight(1).OnPartition("BVN0"))

	// Initiate (and accept) with page 3 with a hold
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			HoldUntil(HoldUntilOptions{MinorBlock: 100}).
			BurnTokens(1, 0).
			SignWith(alice, "book", "3").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Abstain with page 1 (but not enough signatures to reach the threshold)
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Abstain().
			Url(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// Reject with page 2 (overrides page 3's vote)
	sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Reject().
			Url(alice, "book", "2").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// Step until the threshold
	sim.StepN(100 - int(sim.S.BlockIndex(Directory)))

	// Bump
	sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "bump").
			WriteData().DoubleHash().
			SignWith(alice, "book", "2").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// The transaction is rejected (page 2 wins)
	sim.StepUntil(
		Txn(st.TxID).Fails().
			WithError(errors.Rejected))
}

func TestRejectSignaturesAfterExecution(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey))

	// Transaction completes
	sim.StepUntil(
		Txn(st.TxID).Completes())

	// Sign again
	sig := sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey))[1]

	// Authority signature is rejected
	sim.StepUntil(
		Sig(sig.TxID).AuthoritySignature().Fails().
			WithError(errors.NotAllowed).
			WithMessagef("%v has not been initiated", st.TxID))
}

// TestExpiration verifies that block holds work as advertized.
func TestExpiration(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey1 := acctesting.GenerateKey(alice, 1)
	aliceKey2 := acctesting.GenerateKey(alice, 2)

	// Initialize
	g := new(network.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *" // Once a minute
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:], aliceKey2[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e3)})

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
	})

	// Initiate a transaction with an expiration
	env := MustBuild(t,
		build.Transaction().For(alice, "tokens").
			ExpireAtTime(GenesisTime.Add(30*time.Second)).
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))
	st := sim.SubmitSuccessfully(env)

	// It must be pending
	sim.StepUntil(
		Txn(st[0].TxID).IsPending(),
		Sig(st[1].TxID).Succeeds(),
		Sig(st[1].TxID).CreditPayment().Completes())

	// Step past the deadline
	sim.StepUntil(
		BlockTime(GenesisTime.Add(40 * time.Second)))

	// Sign the transaction
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(env.Transaction[0]).
			Url(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey2))

	sim.StepUntil(
		Sig(st[1].TxID).Succeeds())

	// Verify the transaction expired
	sim.Verify(
		Txn(st[0].TxID).Fails().
			WithError(errors.Expired))

	// Initiate another transaction with an expiration
	st = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			ExpireAtTime(GenesisTime.Add((2*60+30)*time.Second)).
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(aliceKey1))

	// It must be pending
	sim.StepUntil(
		Txn(st[0].TxID).IsPending(),
		Sig(st[1].TxID).Succeeds(),
		Sig(st[1].TxID).CreditPayment().Completes())

	// Step until the major block after the expiration
	sim.StepUntilN(1000,
		MajorBlock(3))

	// Verify the transaction expired
	sim.StepUntil(
		Txn(st[0].TxID).Fails().
			WithError(errors.Expired))
}
