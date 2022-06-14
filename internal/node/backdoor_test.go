package node_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestSendDirectToWrongSubnet(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 3, 1, 0, false)
	acctesting.RunTestNet(t, subnets, daemons)
	dn := daemons[protocol.Directory][0]

	// Create the lite addresses and one account
	aliceKey, bobKey := acctesting.GenerateKey("alice"), acctesting.GenerateKey("bob")
	alice, bob := acctesting.AcmeLiteAddressStdPriv(aliceKey), acctesting.AcmeLiteAddressStdPriv(bobKey)

	goodBvnId, err := dn.Jrpc_TESTONLY().Router.RouteAccount(alice)
	require.NoError(t, err)
	goodBvn := daemons[goodBvnId][0]
	_ = goodBvn.DB_TESTONLY().Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(aliceKey), 1e6, 1e9))
		return nil
	})

	// Set route to something else
	var badBvnId string
	for _, subnet := range subnets[1:] {
		if subnet != goodBvnId {
			badBvnId = subnet
			break
		}
	}
	badBvn := daemons[badBvnId][0]

	// Create the transaction
	env := acctesting.NewTransaction().
		WithPrincipal(alice).
		WithSigner(alice, 1).
		WithTimestamp(1).
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{{
				Url:    bob,
				Amount: *big.NewInt(1),
			}},
		}).
		Initiate(protocol.SignatureTypeED25519, aliceKey).
		Build()

	// Submit the transaction directly to the wrong BVN
	local, err := badBvn.LocalClient()
	require.NoError(t, err)
	data, err := env.MarshalBinary()
	require.NoError(t, err)
	result, err := local.BroadcastTxSync(context.Background(), data)
	require.NoError(t, err)
	rset := new(protocol.TransactionResultSet)
	require.NoError(t, rset.UnmarshalBinary(result.Data))
	require.Len(t, rset.Results, 1)
	status := rset.Results[0]
	require.NotZero(t, status.Code)
	require.Equal(t, fmt.Sprintf("signature 0: signature submitted to %s instead of %s", badBvnId, goodBvnId), status.Message)
}
