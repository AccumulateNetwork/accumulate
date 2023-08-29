// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestPendingExpired(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	key1 := acctesting.GenerateKey(1)
	key2 := acctesting.GenerateKey(2)

	g := new(network.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV2
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *"
	g.Globals.Limits = new(NetworkLimits)
	g.Globals.Limits.PendingMajorBlocks = 1

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, key1[32:], key2[32:])
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = FeeCreateToken.AsUInt64()
		p.AcceptThreshold = 2
	})

	// Submit a transaction expensive enough to require a refund
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			CreateToken(alice, "tokens").
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(key1))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Expect the balance to be zero
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"))
	require.Zero(t, page.CreditBalance)

	// Ensure a major block
	sim.StepN(int(time.Minute / time.Second))

	// Transaction expires
	sim.StepUntil(
		Txn(st.TxID).Fails().
			WithError(errors.Expired))

	// Verify state
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		txn := batch.Account(st.TxID.Account()).
			Transaction(st.TxID.Hash())

		v1, err := txn.Payments().Get()
		require.NoError(t, err)
		require.Empty(t, v1)

		v2, err := txn.Votes().Get()
		require.NoError(t, err)
		require.Empty(t, v2)
	})

	// Wait for the refund
	sim.StepUntil(
		Txn(st.TxID).Produced().Completes())

	// Expect the balance to be mostly returned
	page = GetAccount[*KeyPage](t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"))
	require.Equal(t, int(FeeCreateToken-FeeFailedMaximum), int(page.CreditBalance))

	// Sign again
	sig := sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(bob, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(key1).
			SignWith(bob, "book", "1").Timestamp(&timestamp).Version(1).PrivateKey(key2))[2]

	// Authority signature is rejected
	sim.StepUntil(
		Sig(sig.TxID).AuthoritySignature().Fails().
			WithError(errors.NotAllowed).
			WithMessagef("%v has not been initiated", st.TxID))
}

func TestPendingExpiredLimit(t *testing.T) {
	const N = 10

	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey1 := acctesting.GenerateKey(alice, 1)
	aliceKey2 := acctesting.GenerateKey(alice, 2)

	g := new(network.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV2
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *"
	g.Globals.Limits = new(NetworkLimits)
	g.Globals.Limits.PendingMajorBlocks = 1
	g.Globals.Limits.EventsPerBlock = N

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)
	sim.SetRoute(alice, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:], aliceKey2[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
		p.AcceptThreshold = 2
	})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	// Issue transactions
	ids := make([]*url.TxID, N*3)
	cond := make([]Condition, len(ids))
	for i := range ids {
		ids[i] = sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				BurnTokens(1, AcmePrecisionPower).
				SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey1)).TxID

		// Verify they are pending
		cond[i] = Txn(ids[i]).IsPending()
	}
	sim.StepUntil(cond...)

	// Wait for the first major block
	sim.StepUntilN(1000,
		MajorBlock(1))

	// Wait for the expirations to be processed
	sim.StepUntil(True(func(h *Harness) bool {
		for _, id := range ids {
			if Txn(id).Fails().Satisfied(h) {
				return true
			}
		}
		return false
	}))

	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		return a.Compare(b) < 0
	})

	for len(ids) > 0 {
		// The first N transactions expire
		for _, id := range ids[:N] {
			sim.Verify(
				Txn(id).Fails().
					WithError(errors.Expired))
		}

		// The remainder are pending
		ids = ids[N:]
		for _, id := range ids {
			sim.Verify(
				Txn(id).IsPending())
		}

		// Verify state
		View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
			x, err := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Events().Backlog().Expired().Get()
			require.NoError(t, err)
			require.Equal(t, len(ids), len(x))
		})

		// Step once
		sim.Step()
	}
}
