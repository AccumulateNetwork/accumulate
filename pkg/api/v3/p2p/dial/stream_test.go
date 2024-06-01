// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

func TestBatchHandling(t *testing.T) {
	// Start a batch
	ctx1, cancel1, _ := api.ContextWithBatchData(context.Background())
	t.Cleanup(cancel1)

	// Create a context for the request
	ctx2, cancel2 := context.WithCancel(ctx1)
	t.Cleanup(cancel2)

	// Set up a host mock
	ch := make(chan message.Stream, 1)
	host := connectorFunc(func(ctx context.Context, _ *ConnectionRequest) (message.Stream, error) {
		p, q := message.DuplexPipe(ctx, message.QueueDepth(1))
		ch <- q
		return p, nil
	})

	// Dial
	p, err := openStreamFor(ctx2, host, &ConnectionRequest{PeerID: "X", Service: new(api.ServiceAddress)})
	require.NoError(t, err)
	q := <-ch

	// Verify that the stream works
	require.NoError(t, p.Write(&message.EventMessage{}))
	m, err := q.Read()
	require.NoError(t, err)
	require.IsType(t, (*message.EventMessage)(nil), m)

	// Cancel the request context and verify that the stream still works
	require.NoError(t, p.Write(&message.EventMessage{}))
	cancel2()
	m, err = q.Read()
	require.NoError(t, err)
	require.IsType(t, (*message.EventMessage)(nil), m)

	// Cancel the batch context and verify that the stream is closed
	cancel1()
	_, err = q.Read()
	require.ErrorIs(t, err, io.EOF)
}

type connectorFunc func(ctx context.Context, req *ConnectionRequest) (message.Stream, error)

func (f connectorFunc) Connect(ctx context.Context, req *ConnectionRequest) (message.Stream, error) {
	return f(ctx, req)
}
