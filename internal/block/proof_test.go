// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
	req.Url = aliceUrl
	acctResp := simulator.QueryUrl[*api.ChainQueryResponse](sim, aliceUrl, true)
	localReceipt := acctResp.Receipt.Proof
	require.Empty(t, acctResp.Receipt.Error)
	// Execute enough blocks to ensure the block is anchored
	sim.ExecuteBlocks(10)

	// Get a proof of the BVN anchor
	url := protocol.DnUrl().JoinPath(protocol.AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", localReceipt.Anchor))
	chainResp := simulator.QueryUrl[*api.ChainQueryResponse](sim, url, true)
	dirReceipt := chainResp.Receipt.Proof
	fullReceipt, err := localReceipt.Combine(&dirReceipt)
	require.NoError(t, err)
	t.Log(fullReceipt)
}
