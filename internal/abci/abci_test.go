package abci_test

import (
	"log"
	"math/big"
	"testing"

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

	cases := make(map[string]struct {
		Envelope       *protocol.Envelope
		ExpectPriority int
	})
	body := &protocol.SyntheticDepositTokens{
		Token:  protocol.AcmeUrl(),
		Amount: *big.NewInt(100),
	}
	env := newTxn("foo/tokens").
		WithSigner(url.MustParse("foo/book0/1"), 1).
		WithBody(body).
		Initiate(protocol.SignatureTypeLegacyED25519, fooKey).Build()
	cases["syntheticDepositToken"] = struct {
		Envelope       *protocol.Envelope
		ExpectPriority int
	}{
		Envelope:       env,
		ExpectPriority: 1,
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := c.Envelope.MarshalBinary()
			require.NoError(t, err)
			resp := n.app.CheckTx(abci.RequestCheckTx{Tx: b})
			log.Println(resp)
			require.Equal(t, c.ExpectPriority, resp.Priority)
		})
	}
}
