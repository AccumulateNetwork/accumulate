// Copyright 2024 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestPendingTransactionGC(t *testing.T) {
	t.Run("Lite", func(t *testing.T) {
		// Initialize, using pre-Baikonur
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
		)

		aliceKey := acctesting.GenerateKey("alice")
		alice := acctesting.AcmeLiteAddressStdPriv(aliceKey)
		sim.SetRoute(alice.RootIdentity(), "BVN0")

		MakeLiteTokenAccount(t, sim.DatabaseFor(alice), aliceKey[32:], AcmeUrl())

		// Execute
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice).
				BurnTokens(1, 0).
				SignWith(alice).Version(1).Timestamp(1).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Completes())

		// Verify the transaction appears as pending on the LID
		r := sim.QueryPendingIds(alice.RootIdentity(), nil)
		require.Len(sim.TB, r.Records, 1)

		// Update to Baikonur
		st = sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(DnUrl()).
				ActivateProtocolVersion(ExecutorVersionV2Baikonur).
				SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())

		// Give it a few blocks for the anchor to propagate
		sim.StepN(10)

		// Execute maintenance
		st = sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(PartitionUrl("BVN0")).
				Body(&NetworkMaintenance{
					Operations: []NetworkMaintenanceOperation{
						&PendingTransactionGCOperation{
							Account: alice.RootIdentity(),
						},
					},
				}).
				SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 0)))

		sim.StepUntil(
			Txn(st.TxID).Completes())

		// Verify the pending list is empty
		r = sim.QueryPendingIds(alice.RootIdentity(), nil)
		require.Empty(sim.TB, r.Records)
	})

	t.Run("Book", func(t *testing.T) {
		// Initialize, using pre-Baikonur
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
		)

		alice := url.MustParse("alice")
		aliceKey := acctesting.GenerateKey(alice)
		sim.SetRoute(alice, "BVN0")

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e18)
		MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

		// Execute
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "data").
				WriteData().DoubleHash("foo").
				SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st.TxID).Completes())

		// Verify the transaction appears as pending on the book
		r := sim.QueryPendingIds(alice.JoinPath("book"), nil)
		require.Len(sim.TB, r.Records, 1)

		// Update to Baikonur
		st = sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(DnUrl()).
				ActivateProtocolVersion(ExecutorVersionV2Baikonur).
				SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

		sim.StepUntil(
			Txn(st.TxID).Succeeds())

		// Give it a few blocks for the anchor to propagate
		sim.StepN(10)

		// Execute maintenance
		st = sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(PartitionUrl("BVN0")).
				Body(&NetworkMaintenance{
					Operations: []NetworkMaintenanceOperation{
						&PendingTransactionGCOperation{
							Account: alice.JoinPath("book"),
						},
					},
				}).
				SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 0)))

		sim.StepUntil(
			Txn(st.TxID).Completes())

		// Verify the pending list is empty
		r = sim.QueryPendingIds(alice.JoinPath("book"), nil)
		require.Empty(sim.TB, r.Records)
	})

	t.Run("Page", func(t *testing.T) {
		// Initialize, using pre-Baikonur
		g := new(network.GlobalValues)
		g.Globals = new(NetworkGlobals)
		g.Globals.MajorBlockSchedule = "* * * * *" // Once a minute
		g.Globals.Limits = new(NetworkLimits)
		g.Globals.Limits.PendingMajorBlocks = 1
		g.ExecutorVersion = ExecutorVersionV2
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.GenesisWith(GenesisTime, g),
		)

		alice := url.MustParse("alice")
		aliceKey := acctesting.GenerateKey(alice)
		sim.SetRoute(alice, "BVN0")

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e18)
		MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

		// Multisig
		UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
			p.AcceptThreshold = 2
			p.AddKeySpec(&KeySpec{PublicKeyHash: make([]byte, 32)}) // fake key
		})

		// Execute
		st := sim.BuildAndSubmitSuccessfully(
			build.Transaction().For(alice, "data").
				WriteData().DoubleHash("foo").
				SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

		sim.StepUntil(
			Txn(st[0].TxID).IsPending(),
			Sig(st[1].TxID).Succeeds(),
			Sig(st[1].TxID).CreditPayment().Completes())

		// Wait for a major block
		sim.StepUntilN(1000,
			MajorBlock(1))

		// Verify the transaction expires
		sim.StepUntil(
			Txn(st[0].TxID).Fails().
				WithError(errors.Expired))

		// Verify the transaction appears as pending on the book
		r1 := sim.QueryPendingIds(alice.JoinPath("book"), nil)
		require.Len(sim.TB, r1.Records, 1)

		// And there are pending signatures
		r2 := sim.QueryTransaction(st[0].TxID, nil)
		var active int
		for _, sig := range r2.Signatures.Records {
			for _, sig := range sig.Signatures.Records {
				if !sig.Historical {
					active++
				}
			}
		}
		require.NotZero(t, active)

		// Update to Baikonur
		st2 := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(DnUrl()).
				ActivateProtocolVersion(ExecutorVersionV2Baikonur).
				SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))

		sim.StepUntil(
			Txn(st2.TxID).Succeeds())

		// Give it a few blocks for the anchor to propagate
		sim.StepN(10)

		// Execute maintenance
		st2 = sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(PartitionUrl("BVN0")).
				Body(&NetworkMaintenance{
					Operations: []NetworkMaintenanceOperation{
						&PendingTransactionGCOperation{
							Account: alice.JoinPath("book"),
						},
					},
				}).
				SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 0)))

		sim.StepUntil(
			Txn(st2.TxID).Completes())

		// Verify the pending list is empty
		r1 = sim.QueryPendingIds(alice.JoinPath("book"), nil)
		require.Empty(sim.TB, r1.Records)

		// And there are no pending signatures
		r2 = sim.QueryTransaction(st[0].TxID, nil)
		active = 0
		for _, sig := range r2.Signatures.Records {
			for _, sig := range sig.Signatures.Records {
				if !sig.Historical {
					active++
				}
			}
		}
		require.Zero(t, active)
	})
}
