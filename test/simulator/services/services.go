// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package services

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Network struct {
	Services Services
	*message.Client

	// Serving reports whether a peer can answer for the state it holds. A
	// call that names no peer is sent to one that can: see [Network.Dial].
	// Nil means every peer can.
	Serving func(peer.ID) bool
}

type Services map[string]map[peer.ID]Handler

type Handler = func(message.Stream)

func NewNetwork(networkId string, router routing.Router) *Network {
	n := new(Network)
	n.Services = Services{}
	n.Client = &message.Client{Transport: &message.RoutedTransport{
		Network:  networkId,
		Attempts: 1,
		Dialer:   n,
		Router:   &routing.MessageRouter{Router: router},
	}}
	return n
}

func (n *Network) RegisterService(id peer.ID, address *api.ServiceAddress, handler Handler) bool {
	return n.Services.Register(id, address, handler)
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

// Unregister withdraws every service the given peer registered: a stopped
// node answers nothing.
func (s Services) Unregister(id peer.ID) {
	for _, m := range s {
		delete(m, id)
	}
}

// GetHandler returns a handler for the given service address.
func (s Services) GetHandler(address *api.ServiceAddress) Handler {
	m, ok := s[address.String()]
	if !ok {
		return nil
	}
	for _, handler := range m {
		return handler
	}
	return nil
}

// Replace replaces the handler for the given service address and peer ID.
// If the service is not registered, it registers it.
func (s Services) Replace(id peer.ID, address *api.ServiceAddress, handler Handler) {
	m, ok := s[address.String()]
	if !ok {
		m = map[peer.ID]Handler{}
		s[address.String()] = m
	}
	m[id] = handler
}

// Dial implements [message.Dialer] for the simulator's routed client. A call
// that names a peer goes to that peer, whatever its state: a joining node
// answers it, with NotReady for a read, as the daemon's does. A call that
// names none goes to a peer that can answer for its state (Serving).
//
// That is the simulator's stand-in for the daemon's local-first dial: a node's
// routed client answers from the node itself when it serves the partition
// (p2p/dial newNetworkStream), so a validator's own reads never reach a
// joining peer. The simulator has one client for every node and the harness,
// and without this a routed read would land on a joining node at random and
// fail — which no validator's read does. What it does not model: a joining
// node's OWN routed reads, which the daemon answers locally, NotReady.
func (n *Network) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return n.Services.dial(ctx, addr, n.Serving)
}

func (s Services) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return s.dial(ctx, addr, nil)
}

func (s Services) dial(ctx context.Context, addr multiaddr.Multiaddr, serving func(peer.ID) bool) (message.Stream, error) {
	_, peer, sa, _, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if sa == nil {
		return nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	m, ok := s[sa.String()]
	if !ok {
		return nil, errors.NoPeer.WithFormat("no peers found for %v", addr)
	}

	var handler Handler
	if peer != "" {
		handler = m[peer]
	} else {
		for id, h := range m {
			if serving == nil || serving(id) {
				handler = h
				break
			}
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
