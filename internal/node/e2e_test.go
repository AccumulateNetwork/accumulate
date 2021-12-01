package node_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	apitypes "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	tmnet "github.com/tendermint/tendermint/libs/net"
	"github.com/tendermint/tendermint/rpc/client/local"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

func TestEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("This test consistently fails in CI")
	}

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {

		// Restart the nodes for every test
		nodes, dbs := initNodes(s.T(), s.T().Name(), net.ParseIP("127.0.25.1"), 3000, 3, "error", nil)
		query := startNodes(s.T(), nodes)
		client, err := local.New(nodes[0].Service.(local.NodeService))
		require.NoError(s.T(), err)
		return &e2eDUT{s, dbs[0], query, client}
	}))
}

type e2eDUT struct {
	*e2e.Suite
	db     *state.StateDB
	query  *api.Query
	client *local.Local
}

func (d *e2eDUT) GetUrl(url string) (*ctypes.ResultABCIQuery, error) {
	return d.query.QueryByUrl(url)
}

func (d *e2eDUT) SubmitTxn(tx *transactions.GenTransaction) {
	b, err := tx.Marshal()
	d.NoError(err)
	_, err = d.client.BroadcastTxAsync(context.Background(), b)
	d.NoError(err)
}

func (d *e2eDUT) WaitForTxns() {
	// Is there a better way to do this?
	time.Sleep(2 * time.Second)
}

func TestSubscribeAfterClose(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	nodes, _ := initNodes(t, t.Name(), net.ParseIP("127.0.30.1"), 3000, 1, "error", []string{"127.0.30.1"})
	node := nodes[0]
	require.NoError(t, node.Start())
	require.NoError(t, node.Stop())
	node.Wait()

	client, err := local.New(node.Service.(local.NodeService))
	require.NoError(t, err)
	_, err = client.Subscribe(context.Background(), t.Name(), "tm.event = 'Tx'")
	require.EqualError(t, err, "node was stopped")
	time.Sleep(time.Millisecond) // Time for it to panic

	// Ideally, this would also test rpc/core.Environment.Subscribe, but that is
	// not straight forward
}

func TestFaucetMultiNetwork(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("This test does not work well on Windows or macOS")
	}

	bvc0, _ := initNodes(t, "BVC0", net.ParseIP("127.0.26.1"), 3000, 1, "error", []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc1, _ := initNodes(t, "BVC1", net.ParseIP("127.0.27.1"), 3000, 1, "error", []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc2, _ := initNodes(t, "BVC2", net.ParseIP("127.0.28.1"), 3000, 1, "error", []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	rpcAddrs := make([]string, 0, 3)
	wg := new(sync.WaitGroup)
	for _, bvc := range [][]*node.Node{bvc0, bvc1, bvc2} {
		rpcAddrs = append(rpcAddrs, bvc[0].Config.RPC.ListenAddress)
		for _, n := range bvc {
			n := n
			wg.Add(1)
			go func() {
				defer wg.Done()
				require.NoError(t, n.Start())
				t.Cleanup(func() {
					n.Stop()
					n.Wait()
				})
			}()
		}
	}
	wg.Wait()

	relay, err := relay.NewWith(rpcAddrs...)
	require.NoError(t, err)
	if bvc0[0].Config.Accumulate.API.EnableSubscribeTX {
		require.NoError(t, relay.Start())
		t.Cleanup(func() { require.NoError(t, relay.Stop()) })
	}
	query := api.NewQuery(relay)

	lite, err := url.Parse("acc://b5d4ac455c08bedc04a56d8147e9e9c9494c99eb81e9d8c3/ACME")
	require.NoError(t, err)
	require.NotEqual(t, lite.Routing()%3, protocol.FaucetUrl.Routing()%3, "The point of this test is to ensure synthetic transactions are routed correctly. That doesn't work if both URLs route to the same place.")

	req := new(apitypes.APIRequestURL)
	req.Wait = true
	req.URL = types.String(lite.String())

	params, err := json.Marshal(&req)
	require.NoError(t, err)

	port, err := tmnet.GetFreePort()
	require.NoError(t, err)
	jsonapi, err := api.New(&config.API{
		JSONListenAddress: fmt.Sprintf("tcp://localhost:%d", port),
		RESTListenAddress: fmt.Sprintf("tcp://localhost:%d", port+1),
	}, query)
	require.NoError(t, err)
	res := jsonapi.Faucet(context.Background(), params)
	switch r := res.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *apitypes.APIDataResponse:
		// OK
	default:
		require.IsType(t, apitypes.APIDataResponse{}, r)
	}

	// Wait for synthetic TX to settle
	time.Sleep(time.Second)

	obj := new(state.Object)
	chain := new(state.ChainHeader)
	account := new(protocol.LiteTokenAccount)
	qres, err := query.QueryByUrl(lite.String())
	require.NoError(t, err)
	require.Zero(t, qres.Response.Code, "Failed, log=%q, info=%q", qres.Response.Log, qres.Response.Info)
	require.NoError(t, obj.UnmarshalBinary(qres.Response.Value))
	require.NoError(t, obj.As(chain))
	require.Equal(t, types.ChainTypeLiteTokenAccount, chain.Type)
	require.NoError(t, obj.As(account))
}
