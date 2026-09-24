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

	// Client is the nodes' routed client: what every simulated node's
	// executor, conductor and sequencer reach the network through. A call
	// that names no peer goes to any peer that registered the service, a
	// joining one included.
	*message.Client

	// Serving reports whether a peer can answer for the state it holds.
	// Only [Network.HarnessClient] consults it. Nil means every peer can.
	Serving func(peer.ID) bool

	harness *message.Client
}

type Services map[string]map[peer.ID]Handler

type Handler = func(message.Stream)

func NewNetwork(networkId string, router routing.Router) *Network {
	n := new(Network)
	n.Services = Services{}
	n.Client = &message.Client{Transport: &message.RoutedTransport{
		Network:  networkId,
		Attempts: 1,
		Dialer:   n.Services,
		Router:   &routing.MessageRouter{Router: router},
	}}
	n.harness = &message.Client{Transport: &message.RoutedTransport{
		Network:  networkId,
		Attempts: 1,
		Dialer:   harnessDialer{n},
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

// HarnessClient is the client a test's harness reads through: a call that
// names a peer goes to that peer whatever its state, and a call that names none
// goes to a peer that can answer for its state (Serving).
//
// It is the harness's, and only the harness's. It stands in for an operator's
// client connected to a validator's API: the daemon's dialer answers a
// service the node itself provides locally, first (p2p/dial
// newNetworkStream), so such a client's reads never reach a joining peer. The
// daemon has no mechanism that steers a read away from a joining peer: its
// NotReady comes back to the caller as an ErrorResponse, which the client's
// callback accepts (message.typedRequest), so the dial succeeds and neither
// BadDial nor a redial happens. The nodes' own routed calls use Client, which
// can reach a joining peer and be refused, as a non-local call in the daemon
// can.
func (n *Network) HarnessClient() *message.Client { return n.harness }

type harnessDialer struct{ n *Network }

func (d harnessDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return d.n.Services.dial(ctx, addr, d.n.Serving)
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
