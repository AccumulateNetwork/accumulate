package chain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func TestExecutor_Query_ProveAccount(t *testing.T) {
	// Initialize
	router := SetupExecNetwork(t)
	dn := router.Subnet(router.Network.Subnets[0].ID)
	bvn := router.Subnet(router.Network.Subnets[1].ID)
	InitChain(t, dn.Database, dn.Executor)
	InitChain(t, bvn.Database, bvn.Executor)
	router.WaitForGovernor()

	// Create a lite address
	alice := generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	// Fund the lite account
	faucet := protocol.Faucet.Signer()
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithTimestamp(faucet.Timestamp()).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet()
	router.ExecuteBlock(env)
	router.WaitForGovernor()

	// Execute enough blocks to ensure the synthetic transaction has completed
	for i := 0; i < 20; i++ {
		router.ExecuteBlock()
		router.WaitForGovernor()
	}

	// Get a proof of the account state
	req := new(query.RequestByUrl)
	req.Url = types.String(aliceUrl.String())
	acctResp := router.QueryUrl(aliceUrl, req, true).(*query.ResponseAccount)
	localReceipt := acctResp.Receipt.Receipt

	// Execute enough blocks to ensure the block is anchored
	for i := 0; i < 5; i++ {
		router.ExecuteBlock()
		router.WaitForGovernor()
	}

	// Get a proof of the BVN anchor
	req = new(query.RequestByUrl)
	req.Url = types.String(fmt.Sprintf("dn/anchors#anchor/%x", localReceipt.Result))
	chainResp := router.QueryUrl(protocol.DnUrl(), req, true).(*query.ResponseChainEntry)
	dirReceipt := chainResp.Receipt.Receipt

	fullReceipt, err := localReceipt.Convert().Combine(dirReceipt.Convert())
	require.NoError(t, err)
	t.Log(fullReceipt)
}
