package e2e

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestRelaunch(t *testing.T) {
	// Create a network
	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)

	// Start it
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			require.NoError(t, daemon.Start())
			daemon.Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) {
				t.Helper()
				require.NoError(t, err)
			})
		}
	}

	// Create a lite account URL
	lite := url.MustParse("acc://b5d4ac455c08bedc04a56d8147e9e9c9494c99eb81e9d8c3/ACME")

	// Call the faucet
	daemon := daemons[protocol.Directory][0]
	jrpc := daemon.Jrpc_TESTONLY()
	txResp := new(api.TxResponse)
	rpcCall(t, jrpc.Faucet, &protocol.AcmeFaucet{Url: lite}, txResp)
	txqResp := new(api.TransactionQueryResponse)
	rpcCall(t, jrpc.QueryTx, &api.TxnQuery{Txid: txResp.TransactionHash, Wait: 10 * time.Second, IgnorePending: true}, txqResp)
	for _, txid := range txqResp.Produced {
		txid := txid.Hash()
		rpcCall(t, jrpc.QueryTx, &api.TxnQuery{Txid: txid[:], Wait: 10 * time.Second, IgnorePending: true}, nil)
	}

	// Query the account
	account := new(protocol.LiteTokenAccount)
	qResp := new(api.ChainQueryResponse)
	qResp.Data = account
	rpcCall(t, jrpc.Query, &api.UrlQuery{Url: lite}, qResp)
	require.NotZero(t, account.Balance)

	// Stop the network
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			assert.NoError(t, daemon.Stop())
		}
	}

	// Reload and restart it
	var logWriter func(format string) (io.Writer, error)
	if acctesting.LogConsole {
		logWriter = logging.NewConsoleWriter
	} else {
		logWriter = logging.TestLogWriter(t)
	}
	for _, subnet := range subnets {
		daemons := daemons[subnet]
		for i := range daemons {
			dir := daemons[i].Config.RootDir
			var err error
			daemons[i], err = accumulated.Load(dir, func(c *config.Config) (io.Writer, error) { return logWriter(c.LogFormat) })
			require.NoError(t, err)
			daemons[i].Logger = daemons[i].Logger.With("test", t.Name(), "subnet", subnet, "node", i)
		}
	}
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			require.NoError(t, daemon.Start())
			daemon.Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) {
				t.Helper()
				require.NoError(t, err)
			})

			daemon := daemon
			defer func() {
				assert.NoError(t, daemon.Stop())
			}()
		}
	}

	// // Query the account
	// account = new(protocol.LiteTokenAccount)
	// qResp = new(api.ChainQueryResponse)
	// qResp.Data = account
	// rpcCall(t, jrpc.Query, &api.UrlQuery{Url: lite}, qResp)
	// require.NotZero(t, account.Balance)
}

func rpcCall(t *testing.T, method func(context.Context, json.RawMessage) interface{}, input, output interface{}) {
	t.Helper()

	data, err := json.Marshal(input)
	require.NoError(t, err)
	res := method(context.Background(), data)
	if err, ok := res.(error); ok {
		require.NoError(t, err)
	}

	if output == nil {
		return
	}

	data, err = json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, output))
}
