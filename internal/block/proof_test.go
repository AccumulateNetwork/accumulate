package block_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func BenchmarkXxx(b *testing.B) {
	acctesting.EnableDebugFeatures(false)
	defer acctesting.EnableDebugFeatures(true)

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
	require.NoError(b, block.Batch.Commit())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.Batch = x.Database.Begin(true)
		_, err = x.Executor.ExecuteEnvelope(block, env[0])
		block.Batch.Discard()
		require.NoError(b, err)
	}
}

func init() { acctesting.EnableDebugFeatures(true) }

func TestExecutor_Query_ProveAccount(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

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
	req := new(query.RequestByUrl)
	req.Url = types.String(aliceUrl.String())
	acctResp := sim.Query(aliceUrl, req, true).(*query.ResponseAccount)
	localReceipt := acctResp.Receipt.Receipt

	// Execute enough blocks to ensure the block is anchored
	sim.ExecuteBlocks(10)

	// Get a proof of the BVN anchor
	req = new(query.RequestByUrl)
	req.Url = types.String(fmt.Sprintf("dn/anchors#anchor/%x", localReceipt.Result))
	chainResp := sim.Query(protocol.DnUrl(), req, true).(*query.ResponseChainEntry)
	dirReceipt := chainResp.Receipt.Receipt

	fullReceipt, err := localReceipt.Convert().Combine(dirReceipt.Convert())
	require.NoError(t, err)
	t.Log(fullReceipt)
}
