package abci

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestTransactionPriority(t *testing.T) {
	var app *Accumulator
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	nodes := RunTestNet(t, subnets, daemons, nil, true, nil)
	n := nodes[subnets[1]][0]
	app = n.app
	cases := make(map[string]struct {
		Envelope       *protocol.Envelope
		ExpectPriority int
	})
	txns := []*protocol.Transaction{}
	txns = append(txns, &protocol.Transaction{
		Body: &protocol.SyntheticDepositTokens{
			Token:  protocol.AcmeUrl(),
			Amount: *big.NewInt(100),
		},
	})
	cases["syntheticDepositToken"] = struct {
		Envelope       *protocol.Envelope
		ExpectPriority int
	}{
		Envelope: &protocol.Envelope{
			Transaction: txns,
		},
		ExpectPriority: 1,
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := c.Envelope.MarshalBinary()
			require.NoError(t, err)
			resp := app.CheckTx(abci.RequestCheckTx{Tx: b})
			require.Equal(t, c.ExpectPriority, resp.Priority)
		})
	}
}
