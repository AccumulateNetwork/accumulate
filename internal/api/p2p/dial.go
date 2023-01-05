// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"io"
	"sort"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// dialer implements [message.MultiDialer].
type dialer struct {
	host  dialerHost
	peers dialerPeers
}

var _ message.MultiDialer = (*dialer)(nil)

// dialerHost are the parts of [Node] required by [dialer]. dialerHost exists to
// support testing with mocks.
type dialerHost interface {
	selfID() peer.ID
	getOwnService(sa *api.ServiceAddress) (*service, bool)
	getPeerService(ctx context.Context, peer peer.ID, service *api.ServiceAddress) (io.ReadWriteCloser, error)
}

// dialerPeers are the parts of [peerManager] required by [dialer]. dialerPeers
// exists to support testing with mocks.
type dialerPeers interface {
	getPeer(id peer.ID) (*peerState, bool)
	getPeers(service *api.ServiceAddress) []*peerState
	adjustPriority(peer *peerState, delta int)
}

// stream is a [message.Stream] with an associated [peerState].
type stream struct {
	message.Stream
	peer *peerState
}

// Dialer returns a [message.MultiDialer] that knows how to dial any partition
// or node in the network.
func (n *Node) Dialer() message.MultiDialer {
	return dialer{n, n.peermgr}
}

// Dial dials the given address. The address must include an /acc component and
// may include a /p2p component. Dial will return an error if the address
// includes any other components.
//
// If the address is serviceable by the receiving node, the stream will be
// handled locally without requiring any network transport. Otherwise Dial will
// find an appropriate peer that can service the address. If no peer can be
// found, Dial will return [errors.NoPeer].
func (d dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	peer, sa, err := unpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if peer == "" {
		return d.newPartitionStream(ctx, sa)
	}
	return d.newPeerStream(ctx, sa, peer)
}

// unpackAddress unpacks a multiaddr into its components. The address must
// include an /acc component and may include a /p2p component. unpackAddress
// will return an error if the address includes any other components.
func unpackAddress(addr multiaddr.Multiaddr) (peer.ID, *api.ServiceAddress, error) {
	var saBytes, peerID []byte
	var bad bool
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case api.P_ACC:
			saBytes = c.RawValue()
		case multiaddr.P_P2P:
			peerID = c.RawValue()
		default:
			bad = true
		}
		return true
	})

	if bad || saBytes == nil {
		return "", nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	sa := new(api.ServiceAddress)
	err := sa.UnmarshalBinary(saBytes)
	if err != nil {
		return "", nil, errors.BadRequest.WithCauseAndFormat(err, "invalid address %v", addr)
	} else if sa.Type == api.ServiceTypeUnknown {
		return "", nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	return peer.ID(peerID), sa, nil
}

// BadDial notifies the dialer that a transport error was encountered while
// processing the stream.
func (d dialer) BadDial(ctx context.Context, _ multiaddr.Multiaddr, s message.Stream, err error) bool {
	// TODO: Vary the priority hit depending on the type of error
	d.peers.adjustPriority(s.(*stream).peer, -100)

	// TODO: Retry partition connections
	return false
}

// newPeerStream dials the given partition of the given peer. If the peer is the
// current node, newPeerStream returns a pipe and spawns a goroutine to handle
// it as if it were an incoming stream.
//
// If the peer ID does not match a peer known by the node, or if the node does
// not have an address for the given peer, newPeerStream will fail.
func (d dialer) newPeerStream(ctx context.Context, sa *api.ServiceAddress, peer peer.ID) (message.Stream, error) {
	if d.host.selfID() == peer {
		s, ok := d.host.getOwnService(sa)
		if !ok {
			return nil, errors.NotFound // TODO return protocol not supported
		}

		p, q := message.DuplexPipe(ctx)
		go s.handler(p)
		return q, nil
	}

	// Retrieve the peer state
	state, ok := d.peers.getPeer(peer)
	if !ok {
		return nil, errors.BadRequest.WithFormat("unknown peer %v", peer)
	}

	// Open a new stream
	s, err := d.host.getPeerService(ctx, peer, sa)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Close the stream when the context is canceled
	go func() { <-ctx.Done(); _ = s.Close() }()

	ps := new(stream)
	ps.peer = state
	ps.Stream = message.NewStream(s)
	return ps, nil
}

// newPartitionStream opens a stream to the highest priority peer that
// participates in the given partition. If the current node participates in the
// partition, newPartitionStream returns a pipe and spawns a goroutine to handle
// it as if it were an incoming stream.
//
// If the node is aware of multiple peers that participate in the partition, it
// will try them in order of decreasing priority. If a peer is successfully
// dialed, its priority is decremented by one so that subsequent dials are
// routed to a different peer.
func (d dialer) newPartitionStream(ctx context.Context, sa *api.ServiceAddress) (message.Stream, error) {
	// Check if we participate in this partition
	s, ok := d.host.getOwnService(sa)
	if ok {
		p, q := message.DuplexPipe(ctx)
		go s.handler(p)
		return q, nil
	}

	// Sort peers by priority
	peers := d.peers.getPeers(sa)
	sort.Slice(peers, func(i, j int) bool { return peers[i].priority > peers[j].priority })

	// Try each peer in descending priority
	for _, p := range peers {
		// Open a stream
		s, err := d.host.getPeerService(ctx, p.info.ID, sa)
		switch {
		case err == nil:
			// Decrement the priority so we round-robin between peers
			d.peers.adjustPriority(p, -1)

			// Close the stream once the context is done
			go func() { <-ctx.Done(); _ = s.Close() }()

			ps := new(stream)
			ps.peer = p
			ps.Stream = message.NewStream(s)
			return ps, nil

		case errors.Is(err, network.ErrNoConn),
			errors.Is(err, network.ErrNoRemoteAddrs):
			// Can't connect to this peer, decrement the priority and try again.
			// If we were to remove the peer instead, we could have a situation
			// where a network issues lead to us removing all peers. Since that
			// situation is not easily recovered from without restarting the
			// node, we instead decrement the priority by a large number. That
			// way the peer will be avoided unless all other peers have failed.
			d.peers.adjustPriority(p, -100)
			continue

		default:
			return nil, errors.UnknownError.WithFormat("connect to %v of peer: %w", sa, err)
		}
	}
	return nil, errors.NoPeer.WithFormat("no live peers for %v", sa)
}

// selfDialer always dials the [Node] directly.
type selfDialer Node

// SelfDialer returns a [message.Dialer] that always returns a stream for the current node.
func (n *Node) SelfDialer() message.Dialer { return (*selfDialer)(n) }

// Dial returns a stream for the current node.
func (d *selfDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	peer, sa, err := unpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if peer != "" && peer != d.host.ID() {
		return nil, errors.NotFound.With("dialed self but asked for a different peer")
	}

	s, ok := (*Node)(d).getOwnService(sa)
	if !ok {
		return nil, errors.NotFound // TODO return protocol not supported
	}

	p, q := message.DuplexPipe(ctx)
	go s.handler(p)
	return q, nil
}
