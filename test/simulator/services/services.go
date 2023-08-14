// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package services

import (
	"context"
	"runtime/debug"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type Network struct {
	services Services
	*message.Client
}

type Services map[string]map[peer.ID]Handler

type Handler = func(message.Stream)

func NewNetwork(router routing.Router) *Network {
	n := new(Network)
	n.services = Services{}
	n.Client = &message.Client{Transport: &message.RoutedTransport{
		Network:  "Simulator",
		Attempts: 1,
		Dialer:   n.services,
		Router:   &routing.MessageRouter{Router: router},
	}}
	return n
}

func (n *Network) RegisterService(id peer.ID, address *api.ServiceAddress, handler Handler) bool {
	return n.services.Register(id, address, handler)
}

func (s Services) Register(id peer.ID, address *api.ServiceAddress, handler Handler) bool {
	m, ok := s[address.String()]
	if !ok {
		m = map[peer.ID]Handler{}
		s[address.String()] = m
	}

	if _, ok := m[id]; ok {
		return false
	}

	m[id] = handler
	return true
}

func (s Services) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	_, peer, sa, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	m, ok := s[sa.String()]
	if !ok {
		return nil, errors.NoPeer.WithFormat("no peers found for %v", addr)
	}

	var handler Handler
	if peer != "" {
		handler = m[peer]
	} else {
		for _, handler = range m {
			break
		}
	}
	if handler == nil {
		return nil, errors.NoPeer.WithFormat("no peers found for %v", addr)
	}

	p, q := message.DuplexPipe(ctx)
	go func() {
		// Panic protection
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panicked while handling stream", "error", r, "stack", debug.Stack(), "module", "api")
			}
		}()

		defer p.Close()
		handler(p)
	}()
	return q, nil
}
