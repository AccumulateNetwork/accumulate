package jsonrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	testhttp "gitlab.com/accumulatenetwork/accumulate/test/util/http"
)

func setupTest(t testing.TB, services ...Service) *Client {
	handler, err := NewHandler(acctesting.NewTestLogger(t), services...)
	require.NoError(t, err)
	client := new(Client)
	client.Client = *testhttp.DirectHttpClient(handler)
	return client
}

func TestNodeService(t *testing.T) {
	expect := &api.NodeStatus{Ok: true, Version: "asdf", ValidatorKeyHash: [32]byte{1, 2, 3}}
	s := mocks.NewNodeService(t)
	s.EXPECT().NodeStatus(mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, NodeService{NodeService: s})
	actual, err := c.NodeStatus(context.Background(), api.NodeStatusOptions{})
	require.NoError(t, err)
	require.Equal(t, expect, actual)
}

func TestNetworkService(t *testing.T) {
	g := core.NewGlobals(nil)
	expect := &api.NetworkStatus{Oracle: g.Oracle, Globals: g.Globals, Network: g.Network, Routing: g.Routing}
	s := mocks.NewNetworkService(t)
	s.EXPECT().NetworkStatus(mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, NetworkService{NetworkService: s})
	actual, err := c.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	require.NoError(t, err)
	require.Equal(t, expect, actual)
}

func TestMetrics(t *testing.T) {
	expect := &api.Metrics{TPS: 10}
	s := mocks.NewMetricsService(t)
	s.EXPECT().Metrics(mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, MetricsService{MetricsService: s})
	actual, err := c.Metrics(context.Background(), api.MetricsOptions{})
	require.NoError(t, err)
	require.Equal(t, expect, actual)
}

func TestQuerier(t *testing.T) {
	expect := &api.UrlRecord{Value: protocol.AccountUrl("foo")}
	s := mocks.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Querier{Querier: s})
	actual, err := c.Query(context.Background(), nil, nil)
	require.NoError(t, err)
	require.True(t, api.EqualRecord(expect, actual))
}

func TestSubmitter(t *testing.T) {
	expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
	s := mocks.NewSubmitter(t)
	s.EXPECT().Submit(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Submitter{Submitter: s})
	actual, err := c.Submit(context.Background(), nil, api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, len(expect), len(actual))
	require.True(t, expect[0].Equal(actual[0]))
}

func TestValidator(t *testing.T) {
	expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
	s := mocks.NewValidator(t)
	s.EXPECT().Validate(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Validator{Validator: s})
	actual, err := c.Validate(context.Background(), nil, api.ValidateOptions{})
	require.NoError(t, err)
	require.Equal(t, len(expect), len(actual))
	require.True(t, expect[0].Equal(actual[0]))
}
