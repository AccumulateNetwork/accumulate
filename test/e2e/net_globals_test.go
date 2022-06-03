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
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	dn := sim.Subnet(Directory)
	bvn0 := sim.Subnet(sim.Subnets[1].ID)
	// bvn1 := sim.Subnet(sim.Subnets[2].ID)

	// TODO move back to OperatorPage and uncomment extra signatures in or after
	// AC-1402
	signer := simulator.GetAccount[*KeyPage](sim, dn.Executor.Network.ValidatorPage(0))
	_, entry, ok := signer.EntryByKey(dn.Executor.Key[32:])
	require.True(t, ok)
	timestamp = entry.GetLastUsedOn()

	// Update
	price := 445.00
	g := new(core.GlobalValues)
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
			// Sign(SignatureTypeED25519, bvn0.Executor.Key).
			// Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Build(),
	)...)

	// Give it a few blocks for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify account
	account := simulator.GetAccount[*DataAccount](sim, bvn0.Executor.Network.NodeUrl(Oracle))
	require.NotNil(t, account.Entry)
	require.Equal(t, oracleEntry.GetData(), account.Entry.GetData())
	require.Len(t, account.Entry.GetData(), 1)

	// Verify globals variable
	expected := uint64(price * AcmeOraclePrecision)
	require.Equal(t, int(expected), int(dn.Executor.ActiveGlobals_TESTONLY().Oracle.Price))
	require.Equal(t, int(expected), int(bvn0.Executor.ActiveGlobals_TESTONLY().Oracle.Price))
}
