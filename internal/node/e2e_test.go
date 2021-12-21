package node_test

import (
	"context"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"net"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	apiv2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/rpc/client/local"
)

func TestEndToEnd(t *testing.T) {
	acctesting.SkipCI(t, "flaky")
	acctesting.SkipPlatform(t, "windows", "flaky")
	acctesting.SkipPlatform(t, "darwin", "flaky, requires setting up localhost aliases")

	suite.Run(t, e2e.NewSuite(func(s *e2e.Suite) e2e.DUT {
		// Restart the nodes for every test
		nodes := initNodes(s.T(), s.T().Name(), net.ParseIP("127.0.25.1"), 3000, 3, nil)
		bvn0 := nodes[0]
		client, err := local.New(bvn0.Node_TESTONLY().Service.(local.NodeService))
		require.NoError(s.T(), err)
		return &e2eDUT{s, bvn0.DB_TESTONLY(), bvn0.Query_TESTONLY(), client, bvn0.ConnRouter}
	}))
}

type e2eDUT struct {
	*e2e.Suite
	db         *state.StateDB
	query      *api.Query
	client     *local.Local
	connRouter connections.ConnectionRouter
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
	route, err := d.connRouter.GetLocalRoute()
	d.Require().NoError(err)
	q := apiv2.NewQueryDirect(route, apiv2.QuerierOptions{
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
