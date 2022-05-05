package block_test

import (
	"fmt"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func init() { acctesting.EnableDebugFeatures() }

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

	//fullReceipt, err := localReceipt.Convert().Combine(dirReceipt.Convert())
	fullReceipt := localReceipt.Combine(&dirReceipt)
	//	require.NoError(t, err)
	fmt.Println(fullReceipt)
	t.Log(fullReceipt)
}
