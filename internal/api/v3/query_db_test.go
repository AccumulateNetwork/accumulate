package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

var delivered = (*protocol.TransactionStatus).Delivered

func TestDatabaseQueryLayer_QueryState(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Create a lite address
	alice := acctesting.GenerateTmKey(t.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	// Fund the lite account
	faucet := protocol.Faucet.Signer()
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithTimestamp(faucet.Timestamp()).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	// Get a proof of the account state
	x := sim.PartitionFor(aliceUrl)
	dbql := &api.DatabaseQueryModule{Network: &x.Executor.Describe, DB: x.Database}
	rec, err := dbql.QueryState(context.Background(), aliceUrl, nil, api.QueryStateOptions{Prove: true})
	require.NoError(t, err)
	require.IsType(t, (*api.AccountRecord)(nil), rec)
	arec := rec.(*api.AccountRecord)
	require.IsType(t, (*protocol.LiteTokenAccount)(nil), arec.Account)
	lite := arec.Account.(*protocol.LiteTokenAccount)

	// Verify the account
	require.Equal(t, aliceUrl.String(), lite.Url.String())
	require.Equal(t, protocol.AcmeUrl().String(), lite.TokenUrl.String())
	require.Equal(t, uint64(protocol.AcmePrecision*protocol.AcmeFaucetAmount), lite.Balance.Uint64())

	// Validate the proof
	require.True(t, arec.Proof.Proof.Validate())
}
