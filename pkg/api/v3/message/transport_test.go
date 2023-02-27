// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func TestTransport(t *testing.T) {
	var dialCount int
	didCancel := make(chan struct{})
	var dialer Dialer = dialerFunc(func(ctx context.Context, m multiaddr.Multiaddr) (Stream, error) {
		fmt.Println("Dialing", m)
		dialCount++
		s := Pipe(ctx)
		go func() {
			defer close(didCancel)
			defer s.Close()
			for {
				_, err := s.Read()
				switch {
				case err == nil:
					require.NoError(t, s.Write(&ErrorResponse{}))
				case errors.Is(err, io.EOF),
					errors.Is(err, context.Canceled):
					// Done
					return
				default:
					require.NoError(t, err)
				}
			}
		}()
		return s, nil
	})

	addr, err := multiaddr.NewComponent(api.N_ACC_SVC, "query:foo")
	require.NoError(t, err)

	c := &Client{Network: "foo", Dialer: dialer, Router: routerFunc(func(m Message) (multiaddr.Multiaddr, error) { return addr, nil })}
	err = c.roundTrip(context.Background(), []Message{
		&Addressed{Address: addr},
		&Addressed{Address: addr},
	}, func(res, req Message) error { return nil })
	require.NoError(t, err)

	// Verify the dialer was only used once and the context was canceled
	require.Equal(t, 1, dialCount)
	<-didCancel
}
