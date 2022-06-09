package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestOracleDistribution(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Subnet(Directory)
	bvn0 := sim.Subnet(sim.Subnets[1].ID)
	bvn1 := sim.Subnet(sim.Subnets[2].ID)

	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Network.DefaultOperatorPage())
	_, entry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()

	// Update
	price := 445.00
	g = new(core.GlobalValues)
	g.Oracle = new(AcmeOracle)
	g.Oracle.Price = uint64(price * AcmeOraclePrecision)
	oracleEntry := g.FormatOracle()
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(dn.Executor.Network.NodeUrl(Oracle)).
			WithTimestampVar(&timestamp).
			WithSigner(signer.Url, signer.Version).
			WithBody(&WriteData{
				Entry:        oracleEntry,
				WriteToState: true,
			}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Sign(SignatureTypeED25519, bvn0.Executor.Key).
			Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Build(),
	)...)

	// Give it a few blocks for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify account
	bvn := sim.Subnet(sim.Subnets[1].ID)
	account := simulator.GetAccount[*DataAccount](sim, bvn.Executor.Network.NodeUrl(Oracle))
	require.NotNil(t, account.Entry)
	require.Equal(t, oracleEntry.GetData(), account.Entry.GetData())
	require.Len(t, account.Entry.GetData(), 1)

	// Verify globals variable
	expected := uint64(price * AcmeOraclePrecision)
	require.Equal(t, int(expected), int(dn.Executor.ActiveGlobals_TESTONLY().Oracle.Price))
	require.Equal(t, int(expected), int(bvn.Executor.ActiveGlobals_TESTONLY().Oracle.Price))
}

func TestRoutingDistribution(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(g)
	dn := sim.Subnet(Directory)

	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Network.DefaultOperatorPage())
	_, keyEntry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = keyEntry.GetLastUsedOn()

	// Update
	g = dn.Executor.ActiveGlobals_TESTONLY().Copy()
	g.Routing.Overrides = append(g.Routing.Overrides, RouteOverride{
		Account: AccountUrl("staking"),
		Subnet:  Directory,
	})
	entry := g.FormatRouting()
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(dn.Executor.Network.NodeUrl(Routing)).
			WithTimestampVar(&timestamp).
			WithSigner(signer.Url, signer.Version).
			WithBody(&WriteData{
				Entry:        entry,
				WriteToState: true,
			}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Build(),
	)...)

	// Give it a few blocks for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify account
	bvn := sim.Subnet(sim.Subnets[1].ID)
	account := simulator.GetAccount[*DataAccount](sim, bvn.Executor.Network.NodeUrl(Routing))
	require.NotNil(t, account.Entry)
	require.Equal(t, entry.GetData(), account.Entry.GetData())
	require.Len(t, account.Entry.GetData(), 1)

	// Verify globals variable
	require.True(t, g.Routing.Equal(bvn.Executor.ActiveGlobals_TESTONLY().Routing))
}
