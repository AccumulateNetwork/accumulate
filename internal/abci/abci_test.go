package abci_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestTransactionPriority(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	dn := nodes[subnets[0]][0]
	bvn := nodes[subnets[1]][0]
	fooKey := generateKey()
	_ = bvn.db.Update(func(batch *database.Batch) error {
		require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
		require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
		return nil
	})

	type Case struct {
		Envelope       *protocol.Envelope
		ExpectPriority int64
	}
	cases := map[string]Case{
		"System": {
			Envelope: newTxn(bvn.network.AnchorPool().String()).
				WithSigner(bvn.network.DefaultOperatorPage(), 1).
				WithBody(&protocol.DirectoryAnchor{
					SubnetAnchor: protocol.SubnetAnchor{
						Source:          dn.network.NodeUrl(),
						RootChainIndex:  1,
						MinorBlockIndex: 1,
					},
				}).
				InitiateSynthetic(bvn.network.NodeUrl()).
				Sign(protocol.SignatureTypeLegacyED25519, dn.key.Bytes()).
				Build(),
			ExpectPriority: 2,
		},
		"Synthetic": {
			Envelope: newTxn("foo/tokens").
				WithSigner(bvn.network.DefaultOperatorPage(), 1).
				WithBody(&protocol.SyntheticDepositTokens{
					Token:  protocol.AcmeUrl(),
					Amount: *big.NewInt(100),
				}).
				InitiateSynthetic(bvn.network.NodeUrl()).
				Sign(protocol.SignatureTypeLegacyED25519, bvn.exec.Key).
				SignFunc(func(txn *protocol.Transaction) protocol.Signature {
					// Add a receipt signature
					sig := new(protocol.ReceiptSignature)
					sig.SourceNetwork = url.MustParse("foo.acme")
					sig.TransactionHash = *(*[32]byte)(txn.GetHash())
					sig.Proof.Start = txn.GetHash()
					sig.Proof.Anchor = txn.GetHash()
					return sig
				}).
				Build(),
			ExpectPriority: 1,
		},
		"User": {
			Envelope: newTxn("foo.acme/tokens").
				WithSigner(url.MustParse("foo.acme/book0/1"), 1).
				WithBody(&protocol.BurnTokens{
					Amount: *big.NewInt(100),
				}).
				Initiate(protocol.SignatureTypeLegacyED25519, fooKey).
				Build(),
			ExpectPriority: 0,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// Submit the envelope
			b, err := c.Envelope.MarshalBinary()
			require.NoError(t, err)
			resp := bvn.app.CheckTx(abci.RequestCheckTx{Tx: b, Type: abci.CheckTxType_Recheck})
			assert.Zero(t, resp.Code, resp.Log)

			// Check the results
			results := new(protocol.TransactionResultSet)
			require.NoError(t, results.UnmarshalBinary(resp.Data))
			for _, status := range results.Results {
				if status.Error != nil {
					assert.NoError(t, status.Error)
				} else {
					assert.Zero(t, status.Code, status.Message)
				}
			}
			if t.Failed() {
				return
			}

			// Verify the priority
			require.Equal(t, c.ExpectPriority, resp.Priority)
		})
	}
}

func TestCheckTx_SharedBatch(t *testing.T) {
	t.Skip("https://accumulate.atlassian.net/browse/AC-1702")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]

	alice, bob := generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	_ = n.db.Update(func(batch *database.Batch) error {
		require.NoError(n.t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, float64(protocol.FeeSendTokens)/protocol.CreditPrecision))
		return nil
	})

	// Check a transaction
	resp := n.CheckTx(newTxn(aliceUrl.String()).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&protocol.SendTokens{To: []*protocol.TokenRecipient{{
			Url:    bobUrl,
			Amount: *big.NewInt(1),
		}}}).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build())

	// The first transaction should succeed
	require.Zero(t, resp.Code)

	// Check another transaction
	resp = n.CheckTx(newTxn(aliceUrl.String()).
		WithSigner(aliceUrl.RootIdentity(), 1).
		WithBody(&protocol.SendTokens{To: []*protocol.TokenRecipient{{
			Url:    bobUrl,
			Amount: *big.NewInt(1),
		}}}).
		Initiate(protocol.SignatureTypeLegacyED25519, alice).
		Build())

	// The second transaction should fail due to insufficient credits
	require.NotZero(t, resp.Code)
}
