// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
)

func TestOracleDistribution(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest
	sim := simulator.New(t, 3)
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

	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Describe.OperatorsPage())
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
	account := simulator.GetAccount[*DataAccount](sim, bvn.Executor.Describe.NodeUrl(Oracle))
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
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Partition(Directory)

	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Describe.OperatorsPage())
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
	account := simulator.GetAccount[*DataAccount](sim, bvn.Executor.Describe.NodeUrl(Routing))
	require.NotNil(t, account.Entry)
	require.Equal(t, entry.GetData(), account.Entry.GetData())
	require.Len(t, account.Entry.GetData(), 1)

	// Verify globals variable
	require.True(t, g.Routing.Equal(bvn.Globals().Routing))
}
