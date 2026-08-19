// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"slices"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (n *Node) Tracker() dial.Tracker { return n.tracker }

func (n *Node) Connector() dial.Connector { return (*connector)(n) }

// DialNetwork returns a [message.MultiDialer] that opens a stream to a node
// that can provides a given service.
func (n *Node) DialNetwork() message.Dialer {
	var host dial.Connector
	var peers dial.Discoverer

	tr, ok := n.tracker.(*dial.PersistentTracker)
	if ok {
		// Use the persistent tracker as the host and for discovery
		host, peers = tr, tr
	} else {
		// Use the basic host and discovery
		host = (*connector)(n)
		peers = (*dhtDiscoverer)(n)
	}

	// Wrap with connected peers discoverer for fast local discovery
	// This checks connected peers before falling back to DHT
	peers = &connectedPeersDiscoverer{n, peers}

	// Always use self-discovery
	peers = &selfDiscoverer{n, peers}

	return dial.New(
		dial.WithConnector(host),
		dial.WithDiscoverer(peers),
		dial.WithTracker(n.tracker),
	)
}

type selfDiscoverer struct {
	n *Node
	d dial.Discoverer
}

func (d *selfDiscoverer) Discover(ctx context.Context, req *dial.DiscoveryRequest) (dial.DiscoveryResponse, error) {
	if req.Service == nil {
		return d.d.Discover(ctx, req)
	}

	local, ok := d.DiscoverLocal(req.Network, req.Service)
	if !ok {
		return d.d.Discover(ctx, req)
	}

	return local, nil
}

// DiscoverLocal implements [dial.LocalDiscoverer], letting the dialer check
// whether this node provides the service before it issues a network query.
func (d *selfDiscoverer) DiscoverLocal(network string, service *api.ServiceAddress) (dial.DiscoveredLocal, bool) {
	s, ok := d.n.getOwnService(network, service)
	if !ok {
		return nil, false
	}

	return dial.DiscoveredLocal(func(ctx context.Context) (message.Stream, error) {
		return handleLocally(ctx, s), nil
	}), true
}

type dhtDiscoverer Node

func (d *dhtDiscoverer) Discover(ctx context.Context, req *dial.DiscoveryRequest) (dial.DiscoveryResponse, error) {
	var addr multiaddr.Multiaddr
	if req.Network != "" {
		c, err := multiaddr.NewComponent(api.N_ACC, req.Network)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("create network multiaddr: %w", err)
		}
		addr = c
	}
	if req.Service != nil {
		if req.Service.Type == api.ServiceTypeUnknown {
			return nil, errors.BadRequest.With("missing service type")
		}
		c := req.Service.Multiaddr()
		if addr == nil {
			addr = c
		} else {
			addr = addr.Encapsulate(c)
		}
	}
	if addr == nil {
		return nil, errors.BadRequest.With("no network or service specified")
	}

	ch, err := (*Node)(d).peermgr.getPeers(ctx, addr, req.Limit, req.Timeout)
	return dial.DiscoveredPeers(ch), err
}

// connectedPeersDiscoverer checks connected peers before falling back to DHT.
// This provides fast discovery for small networks where DHT propagation may be slow.
type connectedPeersDiscoverer struct {
	n   *Node
	dht dial.Discoverer
}

func (d *connectedPeersDiscoverer) Discover(ctx context.Context, req *dial.DiscoveryRequest) (dial.DiscoveryResponse, error) {
	if req.Service == nil {
		return d.dht.Discover(ctx, req)
	}

	// Build the protocol ID for this service
	protocolID := idRpc(req.Service)

	// Check connected peers for ones that support this protocol
	var found []peer.AddrInfo
	for _, p := range d.n.host.Network().Peers() {
		// Check if peer supports the service protocol
		protocols, err := d.n.host.Peerstore().GetProtocols(p)
		if err != nil {
			continue
		}
		if slices.Contains(protocols, protocolID) {
			addrs := d.n.host.Peerstore().Addrs(p)
			found = append(found, peer.AddrInfo{ID: p, Addrs: addrs})
		}
	}

	// If we found connected peers with the service, return them
	if len(found) > 0 {
		ch := make(chan peer.AddrInfo, len(found))
		for _, p := range found {
			ch <- p
		}
		close(ch)
		return dial.DiscoveredPeers(ch), nil
	}

	// Fall back to DHT discovery
	return d.dht.Discover(ctx, req)
}

type connector Node

func (c *connector) Connect(ctx context.Context, req *dial.ConnectionRequest) (message.Stream, error) {
	if req.PeerID != c.host.ID() {
		return (*Node)(c).getPeerService(ctx, req.PeerID, req.Service, req.PeerAddr)
	}

	s, ok := (*Node)(c).getOwnService("", req.Service)
	if !ok {
		return nil, errors.NotFound // TODO return protocol not supported
	}

	return handleLocally(ctx, s), nil
}
