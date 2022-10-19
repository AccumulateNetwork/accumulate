package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestUnconfirmed queries the node for transactions pending in the mempool and
// decodes them.
func TestUnconfirmed(t *testing.T) {
	addr := "http://bvn0-seed.testnet.accumulatenetwork.io:16592"
	c, err := http.New(addr, addr+"/websocket")
	require.NoError(t, err)

	limit := 100
	res, err := c.UnconfirmedTxs(context.Background(), &limit)
	require.NoError(t, err)

	fmt.Printf("%d unconfirmed transaction(s)\n\n", res.Total)

	for _, tx := range res.Txs {
		env := new(protocol.Envelope)
		err = env.UnmarshalBinary(tx)
		if err != nil {
			t.Errorf("Bad envelope: %v", err)
			continue
		}

		// switch env.Transaction[0].Body.Type() {
		// case protocol.TransactionTypeBlockValidatorAnchor,
		// 	protocol.TransactionTypeDirectoryAnchor:
		// 	continue
		// }

		for _, txn := range env.Transaction {
			fmt.Printf("%v %v\n", txn.Body.Type(), txn.ID())
		}
	}
}
