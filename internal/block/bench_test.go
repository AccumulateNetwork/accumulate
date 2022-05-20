package block_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func BenchmarkXxx(b *testing.B) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	// Initialize the simulator, genesis
	// sim := simulator.NewWithBadger(b, 1, b.TempDir())
	sim := simulator.New(b, 1)
	sim.InitFromGenesis()

	// Create a lite address
	alice := acctesting.GenerateTmKey(b.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	// Start a block
	x := sim.Subnet(sim.Subnets[1].ID)
	block := new(block.Block)
	block.IsLeader = true
	block.Index = 3
	block.Time = time.Now()
	block.Batch = x.Database.Begin(true)
	defer block.Batch.Discard()
	require.NoError(b, x.Executor.BeginBlock(block))

	// // Pre-populate the block with 500 transactions
	// for i := 0; i < 500; i++ {
	// 	env, err := chain.NormalizeEnvelope(acctesting.NewTransaction().
	// 		WithPrincipal(protocol.FaucetUrl).
	// 		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
	// 		Faucet())
	// 	require.NoError(b, err)
	// 	_, err = env[0].LoadTransaction(block.Batch)
	// 	require.NoError(b, err)
	// 	_, err = x.Executor.ExecuteEnvelope(block, env[0])
	// 	require.NoError(b, err)
	// }

	// Construct a new transaction
	env, err := chain.NormalizeEnvelope(acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet())
	require.NoError(b, err)
	_, err = env[0].LoadTransaction(block.Batch)
	require.NoError(b, err)

	// Benchmark ExecuteEnvelope
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block2 := *block // Copy the block
		block2.Batch = block.Batch.Begin(true)
		_, err = x.Executor.ExecuteEnvelope(&block2, env[0])
		block2.Batch.Discard()
		require.NoError(b, err)
	}
}
