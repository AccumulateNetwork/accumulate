// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

// A MessageStreamHandler handles an incoming [message.Stream].
type MessageStreamHandler func(message.Stream)

// idRpc constructs a [protocol.ID] for the given partition.
func idRpc(sa *api.ServiceAddress) protocol.ID {
	return "/acc/rpc/" + protocol.ID(sa.String()) + "/1.0.0"
}

// RegisterService registers a service handler and registers the service with
// the network.
func (n *Node) RegisterService(sa *api.ServiceAddress, handler MessageStreamHandler) bool {
	ptr, ok := sortutil.BinaryInsert(&n.services, func(s *serviceHandler) int { return s.address.Compare(sa) })
	if !ok {
		return false
	}
	*ptr = &serviceHandler{sa, handler}

	n.host.SetStreamHandler(idRpc(sa), func(s network.Stream) {
		// Panic protection
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panicked while handling stream", "error", r, "stack", debug.Stack(), "module", "api")
			}
		}()

		defer s.Close()
		handler(message.NewStream(s))
	})

	err := n.peermgr.advertizeNewService(sa)
	if err != nil {
		slog.Error("Advertizing failed", "error", err, "module", "api")
	}
	return true
}

// serviceHandler manages a [Node]'s participation in a serviceHandler.
type serviceHandler struct {
	address *api.ServiceAddress
	handler MessageStreamHandler
}

// WaitForService IS NOT RELIABLE.
//
// WaitForService blocks until the given service is available. WaitForService
// will return once the service is registered on the current node or until the
// node is informed of a peer with the given service. WaitForService will return
// immediately if the service is already registered or known.
func (s *Node) WaitForService(ctx context.Context, addr multiaddr.Multiaddr) error {
	return s.peermgr.waitFor(ctx, addr)
}

type nodeService Node

func (n *nodeService) NodeInfo(ctx context.Context, opts api.NodeInfoOptions) (*api.NodeInfo, error) {
	info := new(api.NodeInfo)
	info.PeerID = n.host.ID()
	info.Network = n.peermgr.network
	info.Services = make([]*api.ServiceAddress, len(n.services))
	for i, s := range n.services {
		info.Services[i] = s.address
	}
	return info, nil
}

func (n *nodeService) FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	var addr multiaddr.Multiaddr
	if opts.Network != "" {
		c, err := multiaddr.NewComponent(api.N_ACC, opts.Network)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("create network multiaddr: %w", err)
		}
		addr = c
	}
	if opts.Service != nil {
		if opts.Service.Type == api.ServiceTypeUnknown {
			return nil, errors.BadRequest.With("missing service type")
		}
		c := opts.Service.Multiaddr()
		if addr == nil {
			addr = c
		} else {
			addr = addr.Encapsulate(c)
		}
	}
	if addr == nil {
		return nil, errors.BadRequest.With("no network or service specified")
	}

	var results []*api.FindServiceResult
	if opts.Known {
		// Find known peers
		results = n.getKnownPeers(ctx, addr)

	} else {
		// Discover peers using the DHT
		var err error
		results, err = n.discoverPeers(ctx, addr, opts.Timeout)
		if err != nil {
			return nil, err
		}
	}

	// Return an empty array, not nil, because JSON-RPC handles that better
	if results == nil {
		return []*api.FindServiceResult{}, nil
	}

	// Add addresses
	for _, r := range results {
		r.Addresses = n.host.Peerstore().Addrs(r.PeerID)
	}
	return results, nil
}

func (n *nodeService) getKnownPeers(ctx context.Context, addr multiaddr.Multiaddr) []*api.FindServiceResult {
	// Find known peers
	var results []*api.FindServiceResult
	for _, peer := range n.tracker.All(addr, api.PeerStatusIsKnownGood) {
		results = append(results, &api.FindServiceResult{
			PeerID: peer,
			Status: api.PeerStatusIsKnownGood,
		})
	}
	for _, peer := range n.tracker.All(addr, api.PeerStatusIsKnownBad) {
		results = append(results, &api.FindServiceResult{
			PeerID: peer,
			Status: api.PeerStatusIsKnownBad,
		})
	}
	return results
}

func (n *nodeService) discoverPeers(ctx context.Context, addr multiaddr.Multiaddr, timeout time.Duration) ([]*api.FindServiceResult, error) {
	if timeout == 0 {
		timeout = 2 * time.Second
	}

	ch, err := n.peermgr.getPeers(ctx, addr, 100, timeout)
	if err != nil {
		return nil, err
	}

	results := []*api.FindServiceResult{}
	for peer := range ch {
		results = append(results, &api.FindServiceResult{
			PeerID: peer.ID,
			Status: n.tracker.Status(peer.ID, addr),
		})
	}
	return results, nil
}
