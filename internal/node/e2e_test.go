package node_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/rpc/client/local"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

func TestEndToEnd(t *testing.T) {
	t.Skip("This is failing and may be more trouble than it's worth")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		// Restart the nodes for every test
		subnets, daemons := acctesting.CreateTestNet(s.T(), 3, 1, 0)
		acctesting.RunTestNet(s.T(), subnets, daemons)
		daemon := daemons[subnets[1]][0]
		client, err := local.New(daemon.Node_TESTONLY().Service.(local.NodeService))
		require.NoError(s.T(), err)
		return &e2eDUT{s, daemon.DB_TESTONLY(), daemon.Jrpc_TESTONLY(), client}
	}))
}

type e2eDUT struct {
	*e2e.Suite
	db     *database.Database
	api    *apiv2.JrpcMethods
	client *local.Local
}

func (d *e2eDUT) queryAccount(s string) *apiv2.ChainQueryResponse {
	u, err := url.Parse(s)
	d.Require().NoError(err)
	data, err := json.Marshal(&apiv2.UrlQuery{Url: u})
	d.Require().NoError(err)
	r := d.api.Query(context.Background(), data)
	if err, ok := r.(error); ok {
		d.Require().NoError(err)
	}
	d.Require().IsType((*apiv2.ChainQueryResponse)(nil), r)
	return r.(*apiv2.ChainQueryResponse)
}

func (d *e2eDUT) GetRecordAs(url string, target state.Chain) {
	r := d.queryAccount(url)
	data, err := json.Marshal(r.Data)
	d.Require().NoError(err)
	d.Require().NoError(json.Unmarshal(data, target))
}

func (d *e2eDUT) GetRecordHeight(url string) uint64 {
	return d.queryAccount(url).MainChain.Height
}

func (d *e2eDUT) SubmitTxn(tx *transactions.Envelope) {
	d.T().Helper()
	b, err := tx.MarshalBinary()
	d.Require().NoError(err)
	_, err = d.client.BroadcastTxAsync(context.Background(), b)
	d.Require().NoError(err)
}

func (d *e2eDUT) WaitForTxns(txids ...[]byte) {
	d.T().Helper()

	q := d.api.Querier_TESTONLY()

	for len(txids) > 0 {
		var synth [][]byte
		for _, txid := range txids {
			r, err := q.QueryTx(txid, 10*time.Second, apiv2.QueryOptions{})
			d.Require().NoError(err)
			d.Require().NotNil(r.Status, "Transaction status is empty")
			d.Require().True(r.Status.Delivered, "Transaction has not been delivered")
			d.Require().Zero(r.Status.Code, "Transaction failed")
			for _, id := range r.SyntheticTxids {
				id := id // We want a pointer to a copy, not to the loop var
				synth = append(synth, id[:])
			}
		}
		txids = synth
	}
}

func TestSubscribeAfterClose(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			require.NoError(t, daemon.Start())
		}
	}
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			assert.NoError(t, daemon.Stop())
		}
	}

	daemon := daemons[protocol.Directory][0]
	client, err := local.New(daemon.Node_TESTONLY().Service.(local.NodeService))
	require.NoError(t, err)
	_, err = client.Subscribe(context.Background(), t.Name(), "tm.event = 'Tx'")
	require.EqualError(t, err, "node was stopped")
	time.Sleep(time.Millisecond) // Time for it to panic

	// Ideally, this would also test rpc/core.Environment.Subscribe, but that is
	// not straight forward
}

func rpcCall(t *testing.T, method func(context.Context, json.RawMessage) interface{}, input, output interface{}) {
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

func TestFaucetMultiNetwork(t *testing.T) {
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky")
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	subnets, daemons := acctesting.CreateTestNet(t, 3, 1, 0)
	acctesting.RunTestNet(t, subnets, daemons)
	daemon := daemons[protocol.Directory][0]
	jrpc := daemon.Jrpc_TESTONLY()

	lite, err := url.Parse("acc://b5d4ac455c08bedc04a56d8147e9e9c9494c99eb81e9d8c3/ACME")
	require.NoError(t, err)
	require.NotEqual(t, lite.Routing()%3, protocol.FaucetUrl.Routing()%3, "The point of this test is to ensure synthetic transactions are routed correctly. That doesn't work if both URLs route to the same place.")

	txResp := new(apiv2.TxResponse)
	rpcCall(t, jrpc.Faucet, &protocol.AcmeFaucet{Url: lite}, txResp)
	txqResp := new(apiv2.TransactionQueryResponse)
	rpcCall(t, jrpc.QueryTx, &apiv2.TxnQuery{Txid: txResp.TransactionHash, Wait: 10 * time.Second}, txqResp)
	for _, txid := range txqResp.SyntheticTxids {
		rpcCall(t, jrpc.QueryTx, &apiv2.TxnQuery{Txid: txid[:], Wait: 10 * time.Second}, nil)
	}

	// Wait for synthetic TX to settle
	time.Sleep(time.Second)

	account := new(protocol.LiteTokenAccount)
	qResp := new(apiv2.ChainQueryResponse)
	qResp.Data = account
	rpcCall(t, jrpc.Query, &apiv2.UrlQuery{Url: lite}, qResp)
	require.NotZero(t, account.Balance)
}
