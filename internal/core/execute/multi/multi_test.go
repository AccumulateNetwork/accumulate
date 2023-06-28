// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestVersionSwitch(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1), // TODO Change to 3 after fixing anchor healing
		simulator.GenesisWith(GenesisTime, g),
	)

	sim.S.SkipProposalCheck(true) // FIXME should not be necessary

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	// Version is unset
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Attempting to use V2 logic fails
	st := sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice).
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "unsupported transaction type: burnCredits")

	// Create a pending transaction
	p := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			CreateDataAccount(alice, "data").WithAuthority("bob.acme", "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(p.TxID).IsPending())

	// Verify the transaction appears on the pending list
	require.NotEmpty(t, sim.QueryAccount(alice, nil).Pending.Records)

	// Update to v1-halt
	sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1Halt).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(&timestamp).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepN(2)

	// Verify the DN has updated and the BVNs have _not_ updated
	require.Equal(t, ExecutorVersionV1Halt, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, protocol.ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, protocol.ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, protocol.ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Before the update makes it to the BVNs,  do something that produces a
	// synthetic transaction
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			AddCredits().WithOracle(InitialAcmeOracle).Spend(10).To(alice, "book", "1").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// Wait until all partitions are updated
	sim.StepUntil(True(func(h *Harness) bool {
		return GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion.HaltV1() &&
			GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion.HaltV1() &&
			GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion.HaltV1() &&
			GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion.HaltV1()
	}))

	// Verify that the pending transaction has been removed from the pending
	// list
	require.Empty(t, sim.QueryAccount(alice, nil).Pending.Records)

	// Verify that the synthetic transaction has not been processed
	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	r := sim.QueryTransaction(st.TxID, nil)
	require.Len(t, r.Produced.Records, 1)
	r = sim.QueryTransaction(r.Produced.Records[0].Value, nil)
	require.False(t, r.Status.Delivered())

	// The synthetic transaction should still succeed
	sim.StepUntil(
		Txn(st.TxID).Produced().Succeeds())

	// Attempting a user transaction fails
	st = sim.SubmitTxn(MustBuild(t,
		build.Transaction().For(alice).
			AddCredits().WithOracle(InitialAcmeOracle).Spend(10).To(alice, "book", "1").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	require.EqualError(t, st.AsError(), "user messages are not being accepted: an upgrade is in progress")

	// Update to v2
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(&timestamp).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for the anchor to propagate
	sim.StepN(10)

	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Attempting to use V2 logic succeeds
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Sign the pending transaction again
	p = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(p.TxID).Load(sim.Query()).
			Url(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(p.TxID).IsPending())
}
