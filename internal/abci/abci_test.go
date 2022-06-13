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
	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, partitions, daemons, nil, true, nil)
	dn := nodes[partitions[0]][0]
	bvn := nodes[partitions[1]][0]
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
					PartitionAnchor: protocol.PartitionAnchor{
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
			resp := bvn.app.CheckTx(abci.RequestCheckTx{Tx: b})
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
