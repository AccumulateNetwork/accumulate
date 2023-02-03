// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestExecutor_Query_ProveAccount(t *testing.T) {
	if !protocol.IsTestNet {
		t.Skip("Faucet")
	}

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
	acctResp := sim.H.QueryAccount(aliceUrl, &api.DefaultQuery{IncludeReceipt: true})
	localReceipt := acctResp.Receipt
	// Execute enough blocks to ensure the block is anchored
	sim.ExecuteBlocks(10)

	// Get a proof of the BVN anchor
	chainResp := sim.H.SearchForAnchor(protocol.DnUrl().JoinPath(protocol.AnchorPool), &api.AnchorSearchQuery{Anchor: localReceipt.Anchor, IncludeReceipt: true})
	require.Len(t, chainResp.Records, 1)
	dirReceipt := chainResp.Records[0].Receipt
	fullReceipt, err := localReceipt.Combine(&dirReceipt.Receipt)
	require.NoError(t, err)
	t.Log(fullReceipt)
}
