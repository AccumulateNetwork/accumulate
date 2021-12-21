package node_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/api"
	apiv2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
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
)

func TestEndToEnd(t *testing.T) {
	acctesting.SkipCI(t, "flaky")
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky, requires setting up localhost aliases")

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {

		// Restart the nodes for every test
		nodes := initNodes(s.T(), s.T().Name(), net.ParseIP("127.0.25.1"), 3000, 3, nil)
		client, err := local.New(nodes[0].Node_TESTONLY().Service.(local.NodeService))
		require.NoError(s.T(), err)
		return &e2eDUT{s, nodes[0].DB_TESTONLY(), nodes[0].Query_TESTONLY(), client}
	}))
}

type e2eDUT struct {
	*e2e.Suite
	db     *state.StateDB
	query  *api.Query
	client *local.Local
}

func (d *e2eDUT) getObj(url string) *state.Object {
	r, err := d.query.QueryByUrl(url)
	d.Require().NoError(err)
	d.Require().Zero(r.Response.Code, "Query failed: %v", r.Response.Info)

	obj := new(state.Object)
	d.Require().Equal([]byte("chain"), r.Response.Key)
	d.Require().NoError(obj.UnmarshalBinary(r.Response.Value))
	return obj
}

func (d *e2eDUT) GetRecordAs(url string, target state.Chain) {
	d.Require().NoError(d.getObj(url).As(target))
}

func (d *e2eDUT) GetRecordHeight(url string) uint64 {
	return d.getObj(url).Height
}

func (d *e2eDUT) SubmitTxn(tx *transactions.GenTransaction) {
	b, err := tx.Marshal()
	d.Require().NoError(err)
	_, err = d.client.BroadcastTxAsync(context.Background(), b)
	d.Require().NoError(err)
}

func (d *e2eDUT) WaitForTxns(txids ...[]byte) {
	q := apiv2.NewQueryDirect(d.client, apiv2.QuerierOptions{
		TxMaxWaitTime: 10 * time.Second,
	})

	for _, txid := range txids {
		_, err := q.QueryTx(txid, 10*time.Second)
		d.Require().NoError(err)
	}
}

func TestSubscribeAfterClose(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")

	daemon := initNodes(t, t.Name(), net.ParseIP("127.0.30.1"), 3000, 1, []string{"127.0.30.1"})[0]
	require.NoError(t, daemon.Stop())

	client, err := local.New(daemon.Node_TESTONLY().Service.(local.NodeService))
	require.NoError(t, err)
	_, err = client.Subscribe(context.Background(), t.Name(), "tm.event = 'Tx'")
	require.EqualError(t, err, "node was stopped")
	time.Sleep(time.Millisecond) // Time for it to panic

	// Ideally, this would also test rpc/core.Environment.Subscribe, but that is
	// not straight forward
}

func TestFaucetMultiNetwork(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")

	bvc0 := initNodes(t, "BVC0", net.ParseIP("127.0.26.1"), 3000, 1, []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc1 := initNodes(t, "BVC1", net.ParseIP("127.0.27.1"), 3000, 1, []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	bvc2 := initNodes(t, "BVC2", net.ParseIP("127.0.28.1"), 3000, 1, []string{"127.0.26.1", "127.0.27.1", "127.0.28.1"})
	rpcAddrs := make([]string, 0, 3)
	for _, bvc := range [][]*accumulated.Daemon{bvc0, bvc1, bvc2} {
		rpcAddrs = append(rpcAddrs, bvc[0].Config.RPC.ListenAddress)
	}

	relay, err := relay.NewWith(nil, rpcAddrs...)
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
		ListenAddress: fmt.Sprintf("tcp://localhost:%d", port),
	}, query)
	require.NoError(t, err)
	res := jsonapi.Faucet(context.Background(), params)
	switch r := res.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *apitypes.APIDataResponse:
		require.NoError(t, acctesting.WaitForTxV1(query, r))
	default:
		require.IsType(t, (*apitypes.APIDataResponse)(nil), r)
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
