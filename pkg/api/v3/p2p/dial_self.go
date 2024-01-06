// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"runtime/debug"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

// selfDialer always dials the [Node] directly.
type selfDialer Node

// DialSelf returns a [message.Dialer] that always returns a stream for the current node.
func (n *Node) DialSelf() message.Dialer { return (*selfDialer)(n) }

// Dial returns a stream for the current node.
func (d *selfDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	// Parse the address
	_, peer, sa, addr, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if sa == nil {
		return nil, errors.BadRequest.With("missing service address")
	}
	if peer != "" && peer != d.host.ID() {
		return (*Node)(d).getPeerService(ctx, peer, sa, addr)
	}

	// Check if we provide the service
	s, ok := (*Node)(d).getOwnService("", sa)
	if !ok {
		return nil, errors.NotFound // TODO return protocol not supported
	}

	// Create a pipe and handle it
	return handleLocally(ctx, s), nil
}

func handleLocally(ctx context.Context, service *serviceHandler) message.Stream {
	p, q := message.DuplexPipe(ctx)
	go func() {
		// Panic protection
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panicked while handling stream", "error", r, "stack", debug.Stack(), "module", "api")
			}
		}()

		defer p.Close()
		service.handler(p)
	}()
	return q
}
