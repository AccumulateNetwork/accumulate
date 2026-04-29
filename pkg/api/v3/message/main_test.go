// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
)

func TestNodeService(t *testing.T) {
	expect := &api.ConsensusStatus{Ok: true, Version: "asdf", ValidatorKeyHash: [32]byte{1, 2, 3}}
	s := mocks.NewConsensusService(t)
	s.EXPECT().ConsensusStatus(mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, ConsensusService{ConsensusService: s})
	actual, err := c.ConsensusStatus(context.Background(), api.ConsensusStatusOptions{NodeID: "QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", Partition: "foo"})
	require.NoError(t, err)
	require.True(t, expect.Equal(actual))
}

func TestNetworkService(t *testing.T) {
	g := core.NewGlobals(nil)
	expect := &api.NetworkStatus{Oracle: g.Oracle, Globals: g.Globals, Network: g.Network, Routing: g.Routing}
	s := mocks.NewNetworkService(t)
	s.EXPECT().NetworkStatus(mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, NetworkService{NetworkService: s})
	actual, err := c.NetworkStatus(context.Background(), api.NetworkStatusOptions{Partition: "foo"})
	require.NoError(t, err)
	require.True(t, expect.Equal(actual))
}

func TestMetrics(t *testing.T) {
	expect := &api.Metrics{TPS: 10}
	s := mocks.NewMetricsService(t)
	s.EXPECT().Metrics(mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, MetricsService{MetricsService: s})
	actual, err := c.Metrics(context.Background(), api.MetricsOptions{Partition: "foo"})
	require.NoError(t, err)
	require.True(t, expect.Equal(actual))
}

func TestQuerier(t *testing.T) {
	expect := &api.UrlRecord{Value: protocol.AccountUrl("foo")}
	s := mocks.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, Querier{Querier: s})
	actual, err := c.Query(context.Background(), protocol.AccountUrl("foo"), nil)
	require.NoError(t, err)
	require.True(t, api.EqualRecord(expect, actual))
}

// TestQuerier_BptPage exercises BptPageQuery round-trip through the
// message-layer codec. Verifies (a) the request's StartHash and Count
// reach the server intact through union marshalling, and (b) the
// BptPageRecord with non-empty Entries returns intact through the
// record union codec.
//
// Bootstrap-v3's enumerate.Run depends on this wire path; without
// this test, a regression in either codec would silently break
// remote bootstrap.
func TestQuerier_BptPage(t *testing.T) {
	wantStart := [32]byte{0xff, 0xff, 0xff}
	wantCount := uint64(7)
	expect := &api.BptPageRecord{
		BptRoot:   [32]byte{0xab, 0xcd},
		NextStart: [32]byte{0x10, 0x20},
		Done:      false,
		Entries: []*api.BptLeafSummary{
			{KeyHash: [32]byte{0x01}, ValueHash: [32]byte{0x11}},
			{KeyHash: [32]byte{0x02}, ValueHash: [32]byte{0x22}},
		},
	}

	var gotQuery api.Query
	var gotScope *url.URL
	s := mocks.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, scope *url.URL, q api.Query) {
			gotScope = scope
			gotQuery = q
		}).
		Return(expect, nil)
	c := SetupTest(t, Querier{Querier: s})

	q2 := api.Querier2{Querier: c}
	scope := protocol.PartitionUrl("Directory")
	actual, err := q2.QueryBptPage(context.Background(), scope, &api.BptPageQuery{
		StartHash: wantStart,
		Count:     wantCount,
	})
	require.NoError(t, err)

	require.Equal(t, scope.String(), gotScope.String())
	q, ok := gotQuery.(*api.BptPageQuery)
	require.True(t, ok, "server received non-BptPageQuery: %T", gotQuery)
	require.Equal(t, wantStart, q.StartHash)
	require.Equal(t, wantCount, q.Count)

	require.Equal(t, expect.BptRoot, actual.BptRoot)
	require.Equal(t, expect.NextStart, actual.NextStart)
	require.Equal(t, expect.Done, actual.Done)
	require.Len(t, actual.Entries, len(expect.Entries))
	for i, e := range expect.Entries {
		require.Equal(t, e.KeyHash, actual.Entries[i].KeyHash)
		require.Equal(t, e.ValueHash, actual.Entries[i].ValueHash)
	}
}

func TestSubmitter(t *testing.T) {
	expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
	s := mocks.NewSubmitter(t)
	s.EXPECT().Submit(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, Submitter{Submitter: s})
	sig := &protocol.ED25519Signature{Signer: protocol.AccountUrl("foo")}
	actual, err := c.Submit(context.Background(), &messaging.Envelope{Signatures: []protocol.Signature{sig}}, api.SubmitOptions{})
	require.NoError(t, err)
	require.Equal(t, len(expect), len(actual))
	require.True(t, expect[0].Equal(actual[0]))
}

func TestValidator(t *testing.T) {
	expect := []*api.Submission{{Success: true, Status: &protocol.TransactionStatus{}}}
	s := mocks.NewValidator(t)
	s.EXPECT().Validate(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, Validator{Validator: s})
	sig := &protocol.ED25519Signature{Signer: protocol.AccountUrl("foo")}
	actual, err := c.Validate(context.Background(), &messaging.Envelope{Signatures: []protocol.Signature{sig}}, api.ValidateOptions{})
	require.NoError(t, err)
	require.Equal(t, len(expect), len(actual))
	require.True(t, expect[0].Equal(actual[0]))
}

func TestEvents(t *testing.T) {
	expect := []api.Event{
		&api.ErrorEvent{Err: errors.BadRequest.With("foo")},
		&api.BlockEvent{},
		&api.GlobalsEvent{},
	}
	ch := make(chan api.Event)
	go func() {
		for _, expect := range expect {
			ch <- expect
		}
	}()

	s := mocks.NewEventService(t)
	s.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(ch, nil)
	c := SetupTest(t, EventService{EventService: s})
	ctx, cancel := context.WithCancel(context.Background())
	events, err := c.Subscribe(ctx, api.SubscribeOptions{})
	require.NoError(t, err)

	for _, expect := range expect {
		actual := <-events
		require.True(t, api.EqualEvent(expect, actual))
	}

	cancel()
	_, ok := <-events
	require.False(t, ok)
}

func SetupTest(t testing.TB, services ...Service) *Client {
	handler, err := NewHandler(services...)
	require.NoError(t, err)
	addr, err := multiaddr.NewComponent(api.N_ACC_SVC, "unknown:foo")
	require.NoError(t, err)
	return &Client{&RoutedTransport{
		Network: "foo",
		Router:  routerFunc(func(m Message) (multiaddr.Multiaddr, error) { return addr, nil }),
		Dialer: dialerFunc(func(ctx context.Context, _ multiaddr.Multiaddr) (Stream, error) {
			s := Pipe(ctx)
			go func() { <-ctx.Done(); s.Close() }()
			go handler.Handle(s)
			return s, nil
		}),
	}}
}
