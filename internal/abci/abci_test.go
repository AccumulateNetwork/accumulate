package abci_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestTransactionPriority(t *testing.T) {
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]
	fooKey := generateKey()
	batch := n.db.Begin(true)
	require.NoError(t, acctesting.CreateAdiWithCredits(batch, fooKey, "foo", 1e9))
	require.NoError(t, acctesting.CreateTokenAccount(batch, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, batch.Commit())

	type Case struct {
		Envelope       *protocol.Envelope
		ExpectPriority int64
	}
	cases := make(map[string]Case)
	body := &protocol.SyntheticDepositTokens{
		Token:  protocol.AcmeUrl(),
		Amount: *big.NewInt(100),
	}
	env := newTxn("foo/tokens").
		WithSigner(url.MustParse("foo/book0/1"), 1).
		WithBody(body).
		InitiateSynthetic(n.network.NodeUrl()).
		Sign(protocol.SignatureTypeLegacyED25519, fooKey).Build()

	rsig := new(protocol.ReceiptSignature)
	rsig.SourceNetwork = url.MustParse("foo")
	rsig.TransactionHash = *(*[32]byte)(env.Transaction[0].GetHash())
	rsig.Proof.Start = env.Transaction[0].GetHash()
	rsig.Proof.Anchor = env.Transaction[0].GetHash()

	env.Signatures = append(env.Signatures, rsig)

	cases["syntheticDepositToken"] = Case{
		Envelope:       env,
		ExpectPriority: 1,
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// Submit the envelope
			b, err := c.Envelope.MarshalBinary()
			require.NoError(t, err)
			resp := n.app.CheckTx(abci.RequestCheckTx{Tx: b})
			require.Zero(t, resp.Code, resp.Log)

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
