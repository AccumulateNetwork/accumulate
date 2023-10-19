// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// peerManager manages the peer list and peer discovery for a [Node].
type peerManager struct {
	context     context.Context
	host        host.Host
	network     string
	getServices func() []*serviceHandler
	dht         *dht.IpfsDHT
	routing     *routing.RoutingDiscovery
	sendEvent   chan<- event
	broadcast   chan struct{}
	wait        chan chan struct{}
}

// newPeerManager constructs a new [peerManager] for the given host with the
// given options.
func newPeerManager(ctx context.Context, host host.Host, getServices func() []*serviceHandler, opts Options) (*peerManager, error) {
	// Setup the basics
	m := new(peerManager)
	m.host = host
	m.context = ctx
	m.network = opts.Network
	m.getServices = getServices

	// Setup the DHT
	var err error
	m.dht, err = startDHT(host, ctx, opts.DiscoveryMode, opts.BootstrapPeers)
	if err != nil {
		return nil, err
	}

	m.routing = routing.NewRoutingDiscovery(m.dht)

	// Setup events
	var recvEvent <-chan event
	m.sendEvent, recvEvent, err = startServiceDiscovery(ctx, host)
	if err != nil {
		return nil, err
	}

	// Create an event loop to handle service registration notifications
	m.broadcast = make(chan struct{}, 1)
	m.wait = make(chan chan struct{})
	go func() {
		wait := make(chan struct{})

		for {
			select {
			case <-ctx.Done():
				close(wait)
				return

			case m.wait <- wait:
				// Send the wait channel

			case <-recvEvent:
				// Notify of an event
				close(wait)
				wait = make(chan struct{})

			case <-m.broadcast:
				// Notify of a broadcast
				close(wait)
				wait = make(chan struct{})
			}
		}
	}()

	return m, nil
}

// getPeers queries the DHT for peers that provide the given service.
func (m *peerManager) getPeers(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error) {
	return m.routing.FindPeers(ctx, ma.String(), discovery.Limit(limit))
}

// advertizeNewService advertizes new whoami info to everyone.
func (m *peerManager) advertizeNewService(sa *api.ServiceAddress) error {
	var addr multiaddr.Multiaddr
	var err error
	switch {
	case sa.Type == api.ServiceTypeNode,
		m.network == "":
		addr = sa.Multiaddr()

	default:
		addr, err = sa.MultiaddrFor(m.network)
		if err != nil {
			return errors.UnknownError.WithFormat("format multiaddr: %w", err)
		}
	}

	util.Advertise(m.context, m.routing, addr.String())

	m.sendEvent <- &serviceRegisteredEvent{
		PeerID:  m.host.ID(),
		Network: m.network,
		Address: sa,
	}

	m.broadcast <- struct{}{}
	return nil
}

// waitFor blocks until the node has a peer that provides the given address.
func (m *peerManager) waitFor(ctx context.Context, addr multiaddr.Multiaddr) error {
	for {
		// Stop if we're done
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Get the notification channel for events
		wait := <-m.wait

		// Look for a peer
		ch, err := m.getPeers(ctx, addr, 1)
		if err != nil {
			return err
		}

		// Wait for a response
		select {
		case p := <-ch:
			if p.ID != "" {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}

		// Wait for an event; waiting for a second is a hack to work around the
		// limitations of libp2p's pubsub
		select {
		case <-ctx.Done():
		case <-wait:
		case <-time.After(time.Second):
		}
	}
}
