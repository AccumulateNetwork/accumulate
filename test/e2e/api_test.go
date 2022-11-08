// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMinorBlock_Expand(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	lite := acctesting.GenerateKey(t.Name(), "Lite")
	liteUrl := acctesting.AcmeLiteAddressStdPriv(lite)
	batch := sim.PartitionFor(liteUrl).Database.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(lite), AcmeFaucetAmount, 1e9))
	require.NoError(t, batch.Commit())

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(t.Name(), alice)
	keyHash := sha256.Sum256(aliceKey[32:])

	// Execute something
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(liteUrl.RootIdentity()).
			WithTimestampVar(&timestamp).
			WithSigner(liteUrl.RootIdentity(), 1).
			WithBody(&CreateIdentity{
				Url:        alice,
				KeyHash:    keyHash[:],
				KeyBookUrl: alice.JoinPath("book"),
			}).
			Initiate(SignatureTypeLegacyED25519, lite).
			Build(),
	)...)

	// Call the API
	req := new(api.MinorBlocksQuery)
	req.Url = DnUrl()
	req.TxFetchMode = query.TxFetchModeExpand
	req.Start = 1
	req.Count = 10
	res, err := sim.Partition(Directory).API.QueryMinorBlocks(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, res.Items)

	// Verify the response includes some anchors
	var anchors []*Transaction
	for _, item := range res.Items {
		data, err := json.Marshal(item)
		require.NoError(t, err)
		block := new(api.MinorQueryResponse)
		require.NoError(t, json.Unmarshal(data, block))
		if block.TxCount == 0 {
			continue
		}

		for _, txn := range block.Transactions {
			if txn.Transaction.Body.Type() == TransactionTypeBlockValidatorAnchor {
				anchors = append(anchors, txn.Transaction)
			}
		}
	}
	require.NotEmpty(t, anchors)
}
