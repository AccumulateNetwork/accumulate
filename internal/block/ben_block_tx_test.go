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

func BenchmarkBlockTxs(b *testing.B) {

	//number of transactions can be set
	//	nTxs := []int{100,500}

	sim := simulator.New(b, 1)
	sim.InitChain()
	x := sim.Subnet(sim.Subnets[1].ID)

	alice := acctesting.GenerateTmKey(b.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	block := new(block.Block)
	block.IsLeader = true
	block.Index = 3
	block.Time = time.Now()
	block.Batch = x.Database.Begin(true)
	defer block.Batch.Discard()
	require.NoError(b, x.Executor.BeginBlock(block))

	//benchmark on different transaction numbers for a single block
	//	for _, size := range nTxs {
	//		b.Run(fmt.Sprintf("Number of transactions_%d", size), func(b *testing.B) {
	//			for i := 0; i < size; i++ {
	for i := 0; i < 500; i++ {

		env, err := chain.NormalizeEnvelope(acctesting.NewTransaction().
			WithPrincipal(protocol.FaucetUrl).
			WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
			Faucet())
		require.NoError(b, err)
		require.NoError(b, env[0].LoadTransaction(block.Batch))
		_, err = x.Executor.ExecuteEnvelope(block, env[0])
		require.NoError(b, err)
	}

	b.ResetTimer()

	env, err := chain.NormalizeEnvelope(acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet())
	require.NoError(b, err)
	require.NoError(b, env[0].LoadTransaction(block.Batch))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block2 := *block // Copy the block
		block2.Batch = block.Batch.Begin(true)
		_, err = x.Executor.ExecuteEnvelope(&block2, env[0])
		block2.Batch.Discard()
		require.NoError(b, err)
	}
	//		})
	//	}

}
