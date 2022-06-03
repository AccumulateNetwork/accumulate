package e2e

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestUpdateOracle(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	dn := sim.Subnet(Directory)
	bvn0 := sim.Subnet(sim.Subnets[1].ID)
	// bvn1 := sim.Subnet(sim.Subnets[2].ID)

	// Update
	oracle := new(AcmeOracle)
	oracle.Price = InitialAcmeOracleValue + 1
	oracleData, err := json.Marshal(oracle)
	require.NoError(t, err)
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(dn.Executor.Network.NodeUrl(Globals)).
			WithTimestampVar(&timestamp).
			WithSigner(dn.Executor.Network.DefaultValidatorPage(), 1). // TODO Change to operator page and uncomment extra signatures
			WithBody(&WriteData{
				Entry:        &AccumulateDataEntry{Data: [][]byte{oracleData}},
				WriteToState: true,
			}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			// Sign(SignatureTypeED25519, bvn0.Executor.Key).
			// Sign(SignatureTypeED25519, bvn1.Executor.Key).
			Build(),
	)...)

	// Give it a few blocks for the DN to send its anchor
	sim.ExecuteBlocks(10)

	// Verify the update was pushed
	account := simulator.GetAccount[*DataAccount](sim, bvn0.Executor.Network.NodeUrl(Globals))
	require.NotNil(t, account.Entry)
	require.Len(t, account.Entry.GetData(), 1)
	require.Equal(t, oracleData, account.Entry.GetData()[0])
}
