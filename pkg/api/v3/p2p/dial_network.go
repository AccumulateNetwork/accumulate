// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"

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

	s, ok := d.n.getOwnService(req.Network, req.Service)
	if !ok {
		return d.d.Discover(ctx, req)
	}

	return dial.DiscoveredLocal(func(ctx context.Context) (message.Stream, error) {
		return handleLocally(ctx, s), nil
	}), nil
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
