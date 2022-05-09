package block_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
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
	req := new(query.RequestByUrl)
	req.Url = types.String(aliceUrl.String())
	acctResp := sim.Query(aliceUrl, req, true).(*query.ResponseAccount)
	//	var act *protocol.LiteTokenAccount
	//	act = acctResp.Account.(*protocol.LiteTokenAccount)
	///	fmt.Println("account is", act)
	//	dat, err := act.MarshalBinary()
	//	hash := sha256.Sum256(dat)
	//fmt.Println("Hash is ", hex.EncodeToString(hash[:]))
	localReceipt := acctResp.Receipt.Receipt
	//	fmt.Println(hex.EncodeToString(localReceipt.Result), hex.EncodeToString(localReceipt.Start), localReceipt.Convert())
	// Execute enough blocks to ensure the block is anchored
	sim.ExecuteBlocks(10)

	// Get a proof of the BVN anchor
	req = new(query.RequestByUrl)
	req.Url = types.String(fmt.Sprintf("dn/anchors#anchor/%x", localReceipt.Result))
	chainResp := sim.Query(protocol.DnUrl(), req, true).(*query.ResponseChainEntry)
	dirReceipt := chainResp.Receipt.Receipt
	//	fmt.Println(hex.EncodeToString(dirReceipt.Result), hex.EncodeToString(dirReceipt.Start), dirReceipt.Convert())
	fullReceipt, err := localReceipt.Convert().Combine(dirReceipt.Convert())
	//fr := localReceipt.Combine(&dirReceipt)
	//fmt.Println(hex.EncodeToString(fr.Result), hex.EncodeToString(fr.Start), fr.Convert())
	require.NoError(t, err)
	t.Log(fullReceipt)
}
