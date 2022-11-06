package p2p

import (
	"context"
	"io"
	"sort"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
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
	getSelf(part string) *partition
	newRpcStream(ctx context.Context, peer peer.ID, partition string) (io.ReadWriteCloser, error)
}

// dialerPeers are the parts of [peerManager] required by [dialer]. dialerPeers
// exists to support testing with mocks.
type dialerPeers interface {
	getPeer(id peer.ID) (*peerState, bool)
	getPeers(part string) []*peerState // In order of priority
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

// Dial dials the given address.
func (d dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	// Collect the partition and peer IDs
	var partID, peerID []byte
	var bad bool
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case message.P_ACC:
			partID = c.RawValue()
		case multiaddr.P_P2P:
			peerID = c.RawValue()
		default:
			bad = true
		}
		return true
	})

	if bad || partID == nil {
		return nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	if peerID == nil {
		return d.newPartitionStream(ctx, string(partID))
	}
	return d.newPeerStream(ctx, string(partID), peer.ID(peerID))
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
func (d dialer) newPeerStream(ctx context.Context, partition string, peer peer.ID) (message.Stream, error) {
	partition = strings.ToLower(partition)
	if d.host.selfID() == peer {
		p := d.host.getSelf(partition)
		if p == nil || p.rpc == nil {
			return nil, errors.NotFound // TODO return protocol not supported
		}

		ss := message.Pipe(ctx)
		go p.rpc(ss)
		return ss, nil
	}

	// Retrieve the peer state
	state, ok := d.peers.getPeer(peer)
	if !ok {
		return nil, errors.BadRequest.WithFormat("unknown peer %v", peer)
	}

	// Open a new stream
	s, err := d.host.newRpcStream(ctx, peer, partition)
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
func (d dialer) newPartitionStream(ctx context.Context, partition string) (message.Stream, error) {
	// Check if we participate in this partition
	partition = strings.ToLower(partition)
	p := d.host.getSelf(partition)
	if p != nil && p.rpc != nil {
		ss := message.Pipe(ctx)
		go p.rpc(ss)
		return ss, nil
	}

	// Sort peers by priority
	peers := d.peers.getPeers(partition)
	sort.Slice(peers, func(i, j int) bool { return peers[i].priority > peers[j].priority })

	// Try each peer in descending priority
	for _, p := range peers {
		// Open a stream
		s, err := d.host.newRpcStream(ctx, p.info.ID, partition)
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
			return nil, errors.UnknownError.WithFormat("connect to %s peer: %w", partition, err)
		}
	}
	return nil, errors.NoPeer.WithFormat("no live peers for %s", partition)
}
