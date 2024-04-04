// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package jsonrpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks2 "gitlab.com/accumulatenetwork/accumulate/test/mocks/internal_/api/private"
	mocks1 "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
	testhttp "gitlab.com/accumulatenetwork/accumulate/test/util/http"
)

func setupTest(t testing.TB, services ...Service) *Client {
	handler, err := NewHandler(services...)
	require.NoError(t, err)
	client := new(Client)
	client.Client = *testhttp.DirectHttpClient(handler)
	return client
}

func TestConsensusService(t *testing.T) {
	expect := &api.ConsensusStatus{Ok: true, Version: "asdf", ValidatorKeyHash: [32]byte{1, 2, 3}}
	s := mocks1.NewConsensusService(t)
	s.EXPECT().ConsensusStatus(mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, ConsensusService{ConsensusService: s})
	actual, err := c.ConsensusStatus(context.Background(), api.ConsensusStatusOptions{})
	require.NoError(t, err)
	require.Equal(t, expect, actual)
}

func TestNetworkService(t *testing.T) {
	g := core.NewGlobals(nil)
	expect := &api.NetworkStatus{Oracle: g.Oracle, Globals: g.Globals, Network: g.Network, Routing: g.Routing}
	s := mocks1.NewNetworkService(t)
	s.EXPECT().NetworkStatus(mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, NetworkService{NetworkService: s})
	actual, err := c.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	require.NoError(t, err)
	require.Equal(t, expect, actual)
}

func TestMetrics(t *testing.T) {
	expect := &api.Metrics{TPS: 10}
	s := mocks1.NewMetricsService(t)
	s.EXPECT().Metrics(mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, MetricsService{MetricsService: s})
	actual, err := c.Metrics(context.Background(), api.MetricsOptions{})
	require.NoError(t, err)
	require.Equal(t, expect, actual)
}

func TestQuerier(t *testing.T) {
	expect := &api.UrlRecord{Value: protocol.AccountUrl("foo")}
	s := mocks1.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Querier{Querier: s})
	actual, err := c.Query(context.Background(), nil, nil)
	require.NoError(t, err)
	require.True(t, api.EqualRecord(expect, actual))
}

func TestSubmitter(t *testing.T) {
	expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
	s := mocks1.NewSubmitter(t)
	s.EXPECT().Submit(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Submitter{Submitter: s})
	actual, err := c.Submit(context.Background(), nil, api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, len(expect), len(actual))
	require.True(t, expect[0].Equal(actual[0]))
}

func TestValidator(t *testing.T) {
	expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
	s := mocks1.NewValidator(t)
	s.EXPECT().Validate(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Validator{Validator: s})
	actual, err := c.Validate(context.Background(), nil, api.ValidateOptions{})
	require.NoError(t, err)
	require.Equal(t, len(expect), len(actual))
	require.True(t, expect[0].Equal(actual[0]))
}

func TestEmptyResponse(t *testing.T) {
	s := mocks1.NewNodeService(t)
	s.EXPECT().FindService(mock.Anything, mock.Anything).Return(nil, nil)
	c := setupTest(t, NodeService{NodeService: s})
	actual, err := c.FindService(context.Background(), api.FindServiceOptions{})
	require.NoError(t, err)
	require.Empty(t, actual)
}

func TestSequencer(t *testing.T) {
	expect := &api.MessageRecord[messaging.Message]{ID: url.MustParse("foo/bar").WithTxID([32]byte{1, 2, 3})}
	s := mocks2.NewSequencer(t)
	s.EXPECT().Sequence(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := setupTest(t, Sequencer{Sequencer: s})
	actual, err := c.Private().Sequence(context.Background(), url.MustParse("src"), url.MustParse("dst"), 10, private.SequenceOptions{})
	require.NoError(t, err)
	require.True(t, expect.Equal(actual))
}
