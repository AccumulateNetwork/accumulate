// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
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
	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])

	// Version is unset
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersion(0), GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Attempting to use V2 logic fails
	st := sim.Submit(MustBuild(t,
		build.Transaction().For(DnUrl()).
			Body(&PlaceholderTransaction{}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	require.NotNil(t, st.Error)
	require.EqualError(t, st.Error, "unsupported transaction type: placeholder")

	// Execute
	st = sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1Halt).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for everything to settle
	sim.StepN(10)

	// Execute
	st = sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV2).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give it a few blocks for the anchor to propagate
	sim.StepN(10)

	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN1"), PartitionUrl("BVN1").JoinPath(Ledger)).ExecutorVersion)
	require.Equal(t, ExecutorVersionV2, GetAccount[*SystemLedger](t, sim.Database("BVN2"), PartitionUrl("BVN2").JoinPath(Ledger)).ExecutorVersion)

	// Attempting to use V2 logic succeeds
	st = sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice).
			Body(&PlaceholderTransaction{}).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}
