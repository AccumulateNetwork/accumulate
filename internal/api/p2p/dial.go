// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"io"

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
	getOwnService(network string, sa *api.ServiceAddress) (*service, bool)
	getPeerService(ctx context.Context, peer peer.ID, service *api.ServiceAddress) (io.ReadWriteCloser, error)
}

// dialerPeers are the parts of [peerManager] required by [dialer]. dialerPeers
// exists to support testing with mocks.
type dialerPeers interface {
	getPeers(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error)
}

// stream is a [message.Stream] with an associated [peerState].
type stream struct {
	stream message.Stream
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
	net, peer, sa, err := unpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if peer == "" {
		return d.newNetworkStream(ctx, sa, net)
	}
	return d.newPeerStream(ctx, sa, peer)
}

// unpackAddress unpacks a multiaddr into its components. The address must
// include an /acc-svc component and may include a /p2p component or an /acc
// component. unpackAddress will return an error if the address includes any
// other components.
func unpackAddress(addr multiaddr.Multiaddr) (string, peer.ID, *api.ServiceAddress, error) {
	// Scan the address for /acc, /acc-svc, and /p2p components
	var cNetwork, cService, cPeer *multiaddr.Component
	var bad bool
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case api.P_ACC:
			cNetwork = &c
		case api.P_ACC_SVC:
			cService = &c
		case multiaddr.P_P2P:
			cPeer = &c
		default:
			bad = true
		}
		return true
	})

	// The address must contain a /acc-svc component and must not contain any
	// unexpected components
	if bad || cService == nil {
		return "", "", nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	// Parse the /acc-svc component
	sa := new(api.ServiceAddress)
	err := sa.UnmarshalBinary(cService.RawValue())
	if err != nil {
		return "", "", nil, errors.BadRequest.WithCauseAndFormat(err, "invalid address %v", addr)
	} else if sa.Type == api.ServiceTypeUnknown {
		return "", "", nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	switch {
	case cPeer != nil:
		return "", peer.ID(cPeer.RawValue()), sa, nil
	case cNetwork != nil:
		return string(cNetwork.RawValue()), "", sa, nil
	default:
		return "", "", nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}
}

// BadDial notifies the dialer that a transport error was encountered while
// processing the stream.
func (d dialer) BadDial(ctx context.Context, _ multiaddr.Multiaddr, s message.Stream, err error) bool {
	// TODO: Vary the priority hit depending on the type of error
	// d.peers.adjustPriority(s.(*stream).peer, -100)

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
	// If the peer ID is our ID
	if d.host.selfID() == peer {
		// Check if we have the service
		s, ok := d.host.getOwnService("", sa)
		if !ok {
			return nil, errors.NotFound // TODO return protocol not supported
		}

		// Create a pipe and handle it
		p, q := message.DuplexPipe(ctx)
		go s.handler(p)
		return q, nil
	}

	// Open a new stream
	s, err := d.host.getPeerService(ctx, peer, sa)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Close the stream when the context is canceled
	go func() { <-ctx.Done(); _ = s.Close() }()

	ps := new(stream)
	ps.stream = message.NewStream(s)
	return ps, nil
}

// newNetworkStream opens a stream to the highest priority peer that
// participates in the given partition. If the current node participates in the
// partition, newNetworkStream returns a pipe and spawns a goroutine to handle
// it as if it were an incoming stream.
//
// If the node is aware of multiple peers that participate in the partition, it
// will try them in order of decreasing priority. If a peer is successfully
// dialed, its priority is decremented by one so that subsequent dials are
// routed to a different peer.
func (d dialer) newNetworkStream(ctx context.Context, sa *api.ServiceAddress, net string) (message.Stream, error) {
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Check if we participate in this partition
	service, ok := d.host.getOwnService(net, sa)
	if ok {
		p, q := message.DuplexPipe(ctx)
		go service.handler(p)
		return q, nil
	}

	// Construct an address for the service
	addr, err := sa.MultiaddrFor(net)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Query the DHT for peers that provide the service
	peers, err := d.peers.getPeers(callCtx, addr, 10)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Try each peer in descending priority
	for p := range peers {
		// Open a stream
		s, err := d.host.getPeerService(ctx, p.ID, sa)
		switch {
		case err == nil:
			// Close the stream once the context is done
			go func() { <-ctx.Done(); _ = s.Close() }()

			ps := new(stream)
			ps.stream = message.NewStream(s)
			return ps, nil

		case errors.Is(err, network.ErrNoConn),
			errors.Is(err, network.ErrNoRemoteAddrs):
			// Can't connect to this peer, try again
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
	// Parse the address
	_, peer, sa, err := unpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if peer != "" && peer != d.host.ID() {
		return nil, errors.NotFound.With("dialed self but asked for a different peer")
	}

	// Check if we provide the service
	s, ok := (*Node)(d).getOwnService("", sa)
	if !ok {
		return nil, errors.NotFound // TODO return protocol not supported
	}

	// Create a pipe and handle it
	p, q := message.DuplexPipe(ctx)
	go s.handler(p)
	return q, nil
}

func (s *stream) Read() (message.Message, error) {
	// Convert ErrReset and Canceled into EOF
	m, err := s.stream.Read()
	switch {
	case err == nil:
		return m, nil
	case isEOF(err):
		return nil, io.EOF
	default:
		return nil, err
	}
}

func (s *stream) Write(msg message.Message) error {
	// Convert ErrReset and Canceled into EOF
	err := s.stream.Write(msg)
	switch {
	case err == nil:
		return nil
	case isEOF(err):
		return io.EOF
	default:
		return err
	}
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, network.ErrReset) ||
		errors.Is(err, context.Canceled)
}
