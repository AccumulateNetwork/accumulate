// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	compatSim "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestOracleDistribution(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest
	sim := compatSim.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Partition(Directory)
	bvn0 := sim.Partition("BVN0")
	// bvn1 := sim.Partition("BVN1")

	// Verify that PendingUpdates is never set
	sim.S.SetBlockHook(Directory, func(_ execute.BlockParams, env []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
		ledger := GetAccount[*SystemLedger](t, sim.S.Database(Directory), url.MustParse("dn.acme").JoinPath(Ledger))
		assert.Empty(t, ledger.PendingUpdates)
		return env, true
	})

	signer := compatSim.GetAccount[*KeyPage](sim, dn.Executor.Describe.OperatorsPage())
	_, entry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()

	// Update
	price := 445.00
	g = g.Copy()
	g.Oracle = new(AcmeOracle)
	g.Oracle.Price = uint64(price * AcmeOraclePrecision)
	oracleEntry := g.FormatOracle()
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(dn.Executor.Describe.NodeUrl(Oracle)).
			Body(&WriteData{
				Entry:        oracleEntry,
				WriteToState: true,
			}).
			SignWith(signer.Url).Version(signer.Version).Timestamp(&timestamp).PrivateKey(dn.Executor.Key).
			SignWith(signer.Url).Version(signer.Version).Timestamp(&timestamp).PrivateKey(bvn0.Executor.Key)),
	// Sign(SignatureTypeED25519, bvn1.Executor.Key).

	)...)

	// Give it a few blocks for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify account
	bvn := sim.Partition("BVN0")
	account := compatSim.GetAccount[*DataAccount](sim, bvn.Executor.Describe.NodeUrl(Oracle))
	require.NotNil(t, account.Entry)
	require.Equal(t, oracleEntry.GetData(), account.Entry.GetData())
	require.Len(t, account.Entry.GetData(), 1)

	// Verify globals variable
	expected := uint64(price * AcmeOraclePrecision)
	require.Equal(t, int(expected), int(dn.Globals().Oracle.Price))
	require.Equal(t, int(expected), int(bvn.Globals().Oracle.Price))
}

func TestRoutingDistribution(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest
	sim := compatSim.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Partition(Directory)

	signer := compatSim.GetAccount[*KeyPage](sim, dn.Executor.Describe.OperatorsPage())
	_, keyEntry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = keyEntry.GetLastUsedOn()

	// Update
	g = dn.Globals().Copy()
	g.Routing.Overrides = append(g.Routing.Overrides, RouteOverride{
		Account:   AccountUrl("staking"),
		Partition: Directory,
	})
	entry := g.FormatRouting()
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(dn.Executor.Describe.NodeUrl(Routing)).
			Body(&WriteData{
				Entry:        entry,
				WriteToState: true,
			}).
			SignWith(signer.Url).Version(signer.Version).Timestamp(&timestamp).PrivateKey(dn.Executor.Key)),
	)...)

	// Give it a few blocks for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify account
	bvn := sim.Partition("BVN0")
	account := compatSim.GetAccount[*DataAccount](sim, bvn.Executor.Describe.NodeUrl(Routing))
	require.NotNil(t, account.Entry)
	require.Equal(t, entry.GetData(), account.Entry.GetData())
	require.Len(t, account.Entry.GetData(), 1)

	// Verify globals variable
	require.True(t, g.Routing.Equal(bvn.Globals().Routing))
}

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
