package message

import (
	"context"
	"fmt"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
)

func TestBatchDialer(t *testing.T) {
	expect := &api.UrlRecord{Value: protocol.AccountUrl("foo")}
	s := mocks.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)

	logger := logging.ConsoleLoggerForTest(t, "info")
	handler, err := NewHandler(logger, Querier{Querier: s})
	require.NoError(t, err)

	var dialCount int
	didCancel := make(chan struct{})
	var dialer Dialer = dialerFunc(func(ctx context.Context, m multiaddr.Multiaddr) (Stream, error) {
		fmt.Println("Dialing", m)
		dialCount++
		s := Pipe(ctx)
		go func() {
			defer close(didCancel)
			defer s.Close()
			handler.Handle(s)
		}()
		return s, nil
	})
	batchCtx, batchDone := context.WithCancel(context.Background())
	dialer = BatchDialer(batchCtx, dialer)
	addr, err := multiaddr.NewComponent("acc", "foo")
	require.NoError(t, err)
	client := &Client{Dialer: dialer, Router: routerFunc(func(m Message) (multiaddr.Multiaddr, error) { return addr, nil })}

	_, err = client.Query(context.Background(), protocol.DnUrl(), nil)
	require.NoError(t, err)
	_, err = client.Query(context.Background(), protocol.DnUrl(), nil)
	require.NoError(t, err)

	batchDone()

	// Verify the dialer was only used once and the context was canceled
	require.Equal(t, 1, dialCount)
	<-didCancel
}

type routerFunc func(Message) (multiaddr.Multiaddr, error)

func (fn routerFunc) Route(msg Message) (multiaddr.Multiaddr, error) { return fn(msg) }

type dialerFunc func(context.Context, multiaddr.Multiaddr) (Stream, error)

func (fn dialerFunc) Dial(ctx context.Context, addr multiaddr.Multiaddr) (Stream, error) {
	return fn(ctx, addr)
}
