// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestExecutorVersionDistribution(t *testing.T) {
	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Version is unset
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Execute
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for the anchor to propagate
	sim.StepN(10)

	// Verify version is set
	require.Equal(t, ExecutorVersionV1, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1, GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1, GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV1, GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)
}

// TestDelegatedVersionUpdate verifies that ActivateProtocolVersion can be
// executed with delegated signatures.
func TestDelegatedVersionUpdate(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionV2Vandenberg

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	UpdateAccount(t, sim.Database(Directory), DnUrl().JoinPath(Operators, "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book")})
	})

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionLatest).
			SignWith(alice, "book", "1").Delegator(DnUrl(), Operators, "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}

// TestNoBvnVersionUpdate verifies that a BVN cannot be version-updated
// independently.
func TestNoBvnVersionUpdate(t *testing.T) {
	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionV2Vandenberg

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	bvn := PartitionUrl("BVN0")
	ops := bvn.JoinPath(Operators, "1")
	CreditCredits(t, sim.Database("BVN0"), ops, 1e9)

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(bvn).
			ActivateProtocolVersion(ExecutorVersionLatest).
			SignWith(ops).Version(1).Timestamp(1).Signer(sim.SignWithNode("BVN0", 0)))

	sim.StepUntil(
		Txn(st.TxID).Fails().
			WithError(errors.BadRequest).
			WithMessagef("%v cannot be updated directly", bvn))
}
