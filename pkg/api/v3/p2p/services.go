// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"gitlab.com/accumulatenetwork/accumulate"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
	return n.RegisterServiceIf(sa, handler, nil)
}

// RegisterServiceIf registers a service handler whose OFFER is conditional.
//
// A node offers a service when it advertises it to the DHT, lists it in
// NodeInfo, and lets its own clients dial it locally. It HANDLES a service
// when the stream handler is installed. The two are not the same thing, and
// #4366 is why: a node whose author key is in no committee of a partition
// cannot get a submission into a block — a submission's only road is the
// receiving node's own batch and header, and a header from an author outside
// the committee is dropped before any vote (consensus.md, "What a batch is";
// pkg/consensus/primary/vote_handler.go:277-284). So it must not be found as
// a provider of submit or validate for that partition, and its own API must
// not resolve them to itself, because dialling is local-first
// (dial/dialer.go:146-152) and an unadvertised service still offered locally
// is a dead end for its own clients (executor.md, Sync step 5).
//
// The handler is installed either way. A DHT provider record lingers to its
// TTL whatever the node does, so a peer will dial this node after it stops
// advertising; it must get the service's NotReady, which names the reason,
// rather than a protocol error.
//
// offer is read on every question, not latched at registration — membership
// is a property of the current committee. What is taken once is the
// advertisement itself: util.Advertise re-publishes on its own schedule and
// there is no un-advertise, so a node that gains membership after
// registration serves and lists the service but is not advertised until it
// restarts (phase 2, with #4336's readiness latch).
//
// A nil offer means always offered, which is every caller that predates this.
func (n *Node) RegisterServiceIf(sa *api.ServiceAddress, handler MessageStreamHandler, offer func() bool) bool {
	ptr, ok := sortutil.BinaryInsert(&n.services, func(s *serviceHandler) int { return s.address.Compare(sa) })
	if !ok {
		return false
	}
	*ptr = &serviceHandler{sa, handler, offer}

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

	if offer != nil && !offer() {
		slog.Info("Not advertising a service this node cannot serve",
			"service", sa.String(), "module", "api")
		return true
	}

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

	// offer decides whether the node offers this service — see
	// [Node.RegisterServiceIf]. Nil means always.
	offer func() bool
}

// offered reports whether the node offers this service right now.
func (s *serviceHandler) offered() bool { return s == nil || s.offer == nil || s.offer() }

// Offers reports whether this node offers the given service to the network
// and to its own clients. A service it handles but does not offer answers
// when it is asked and is not advertised.
func (n *Node) Offers(sa *api.ServiceAddress) bool {
	s, ok := n.getOwnService("", sa)
	return ok && s.offered()
}

// handles reports whether the node has a handler for the service, offered or
// not.
func (n *Node) handles(sa *api.ServiceAddress) bool {
	_, ok := n.getOwnService("", sa)
	return ok
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
	info.Services = make([]*api.ServiceAddress, 0, len(n.services))
	info.Version = accumulate.Version
	info.Commit = accumulate.Commit
	for _, s := range n.services {
		// What the node OFFERS, not what it handles: a service it answers
		// only to say NotReady is not one to route work to (#4366).
		if !s.offered() {
			continue
		}
		info.Services = append(info.Services, s.address)
	}
	return info, nil
}

func (n *nodeService) FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	var addr multiaddr.Multiaddr
	// A node advertises a service under its network's key
	// (peer_manager.go, MultiaddrFor), so a search that names no network
	// searches a key nobody advertises and answers "nobody serves that",
	// instantly and without an error. Every caller inside a node means its
	// own network; the ones that did not say so got silence. The join took
	// that silence for "the whole network restarted" and executed from its
	// own empty staging -- the divergence of #4290 -- and the conductor took
	// it for "this source has one validator" and never gathered a second
	// anchor signature on any live network (#4296).
	if opts.Network == "" {
		opts.Network = n.peermgr.network
	}
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
		r.Addresses = dialableAddrs(n.host.Peerstore().Addrs(r.PeerID))
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

// dialableAddrs drops addresses a remote caller cannot use — but only for
// peers that have at least one address it can.
//
// The peerstore holds the addresses by which THIS node reaches a peer, and
// those legitimately include loopback: a node reaches services on itself over
// 127.0.0.1. Returned verbatim to a remote caller they are noise at best and
// misdirection at worst. Measured against mainnet on 2026-08-17, 4 of 10 peers
// advertised for query:Directory were undialable from outside, so a client
// bootstrapping from the list burned ~40% of its dials.
//
// The conditional is what makes this safe. Filtering unconditionally would
// break every local deployment: a netsim separates its nodes by loopback
// address (127.0.1.2/.3/.4, see #4060), so every address it has is
// non-routable and the filter would return nothing at all — turning a cosmetic
// problem into a total discovery failure. When a peer has no routable address,
// its list is passed through unchanged and the caller is no worse off than
// before (#4091).
func dialableAddrs(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	routable := make([]multiaddr.Multiaddr, 0, len(addrs))
	for _, a := range addrs {
		if manet.IsPublicAddr(a) {
			routable = append(routable, a)
		}
	}
	if len(routable) == 0 {
		return addrs
	}
	return routable
}
