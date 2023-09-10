// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"io"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

// dialer implements [message.MultiDialer].
type dialer struct {
	host    dialerHost
	peers   dialerPeers
	tracker peerTracker
	lastTry sync.Map
}

var _ message.MultiDialer = (*dialer)(nil)

// dialerHost are the parts of [Node] required by [dialer]. dialerHost exists to
// support testing with mocks.
type dialerHost interface {
	selfID() peer.ID
	getOwnService(network string, sa *api.ServiceAddress) (*serviceHandler, bool)
	getPeerService(ctx context.Context, peer peer.ID, service *api.ServiceAddress) (io.ReadWriteCloser, error)
}

// dialerPeers are the parts of [peerManager] required by [dialer]. dialerPeers
// exists to support testing with mocks.
type dialerPeers interface {
	getPeers(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error)
}

// DialNetwork returns a [message.MultiDialer] that opens a stream to a node
// that can provides a given service.
func (n *Node) DialNetwork() message.MultiDialer {
	return &dialer{
		host:    n,
		peers:   n.peermgr,
		tracker: n.tracker,
	}
}

// Dial dials the given address. The address must include an /acc component and
// may include a /p2p component. Dial will return an error if the address
// includes any other components.
//
// If the address is serviceable by the receiving node, the stream will be
// handled locally without requiring any network transport. Otherwise Dial will
// find an appropriate peer that can service the address. If no peer can be
// found, Dial will return [errors.NoPeer].
func (d *dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (stream message.Stream, err error) {
	net, peer, sa, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if peer == "" {
		// Do not set the wait group
		return d.newNetworkStream(ctx, sa, net, nil)
	}

	return d.newPeerStream(ctx, sa, peer)
}

// BadDial notifies the dialer that a transport error was encountered while
// processing the stream.
func (d *dialer) BadDial(ctx context.Context, addr multiaddr.Multiaddr, s message.Stream, err error) bool {
	ss, ok := s.(*stream)
	if !ok {
		return false
	}
	slog.InfoCtx(ctx, "Bad dial", "peer", ss.peer, "address", addr, "error", err)
	d.tracker.markBad(ctx, ss.peer, addr)
	return true
}

// newPeerStream dials the given partition of the given peer. If the peer is the
// current node, newPeerStream returns a pipe and spawns a goroutine to handle
// it as if it were an incoming stream.
//
// If the peer ID does not match a peer known by the node, or if the node does
// not have an address for the given peer, newPeerStream will fail.
func (d *dialer) newPeerStream(ctx context.Context, sa *api.ServiceAddress, peer peer.ID) (message.Stream, error) {
	// If the peer ID is our ID
	if d.host.selfID() == peer {
		// Check if we have the service
		s, ok := d.host.getOwnService("", sa)
		if !ok {
			return nil, errors.NotFound // TODO return protocol not supported
		}

		// Create a pipe and handle it
		return handleLocally(ctx, s), nil
	}

	// Open a new stream
	return openStreamFor(ctx, d.host, peer, sa)
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
//
// The wait group is only used for testing.
func (d *dialer) newNetworkStream(ctx context.Context, service *api.ServiceAddress, netName string, wg *sync.WaitGroup) (message.Stream, error) {
	// Check if we participate in this partition
	if h, ok := d.host.getOwnService(netName, service); ok {
		return handleLocally(ctx, h), nil
	}

	// Construct an address for the service
	addr, err := service.MultiaddrFor(netName)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Query the DHT for peers that provide the service
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	peers, err := d.peers.getPeers(callCtx, addr, 10)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Check the remaining unknown peers from the DHT (non-blocking)
	defer func() {
		for i := 0; i < 10; {
			select {
			case peer, ok := <-peers:
				if !ok {
					return
				}

				if d.tracker.status(ctx, peer.ID, addr) != api.PeerStatusIsUnknown {
					break
				}

				i++
				d.tryDial(peer.ID, service, addr, wg)
			default:
				return
			}
		}
	}()

	// If there are at least 4 known-good peers, try those
	if len(d.tracker.allGood(ctx, addr)) >= 4 {
		s := d.dialFromTracker(ctx, service, addr, wg)
		if s != nil {
			return s, nil
		}
	}

	// Try peers from the DHT
	var bad []peer.ID
	for peer := range peers {
		// Skip known-bad peers
		if d.tracker.status(ctx, peer.ID, addr) == api.PeerStatusIsKnownBad {
			bad = append(bad, peer.ID)
			continue
		}

		s := d.dial(ctx, peer.ID, service, addr)
		if s != nil {
			return s, nil
		}
	}

	// If there are any known-good peers, try those
	s := d.dialFromTracker(ctx, service, addr, wg)
	if s != nil {
		return s, nil
	}

	// Retry bad peers as a last ditch effort
	for _, peer := range bad {
		s := d.dial(ctx, peer, service, addr)
		if s != nil {
			return s, nil
		}
	}

	// Give up
	return nil, errors.NoPeer.WithFormat("no live peers for %v", service)
}

func (d *dialer) dialFromTracker(ctx context.Context, service *api.ServiceAddress, addr multiaddr.Multiaddr, wg *sync.WaitGroup) *stream {
	// Asynchronously retry a known-bad peer to check if it has recovered
	if bad, ok := d.tracker.nextBad(ctx, addr); ok {
		d.tryDial(bad, service, addr, wg)
	}

	// Make 10 attempts to find a known-good peer, but only if there are at
	// least 4 known-good peers
	var first peer.ID
	for i := 0; i < 10; i++ {
		// Get the next
		peer, ok := d.tracker.nextGood(ctx, addr)
		if !ok {
			return nil
		}

		// Break the loop if we got back to the beginning
		if first == "" {
			first = peer
		} else if first == peer {
			return nil
		}

		// Dial it
		s := d.dial(ctx, peer, service, addr)
		if s != nil {
			return s
		}
	}

	return nil
}

// tryDial attempts to dial the peer in a goroutine. tryDial is used to maintain
// the peer tracker, not to open a usable stream.
func (d *dialer) tryDial(peer peer.ID, service *api.ServiceAddress, addr multiaddr.Multiaddr, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
	}

	type Attempt struct {
		count atomic.Int32
		time  atomic.Pointer[time.Time]
	}

	// Has it been more than 1 minute since our last attempt?
	v, didLoad := d.lastTry.LoadOrStore(peer, new(Attempt))
	last := v.(*Attempt)
	if didLoad && time.Since(*last.time.Load()) < time.Minute {
		// The last attempt was less than a minute ago
		return

	} else {
		// Update the attempt time and count
		t := time.Now()
		last.count.Add(1)
		last.time.Store(&t)
	}

	go func() {
		if wg != nil {
			defer wg.Done()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		s := d.dial(ctx, peer, service, addr)
		if s != nil {
			s.conn.Close()
		}
	}()
}

func (d *dialer) dial(ctx context.Context, peer peer.ID, service *api.ServiceAddress, addr multiaddr.Multiaddr) *stream {
	// Panic protection
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panicked while dialing a peer", "error", r, "stack", debug.Stack(), "module", "api")
		}
	}()

	// Open a stream
	stream, err := openStreamFor(ctx, d.host, peer, service)
	if err == nil {
		// Mark the peer good
		d.tracker.markGood(ctx, peer, addr)
		return stream
	}

	// Log the error and mark the peer
	switch classifyDialError(ctx, peer, service, addr, err) {
	case severityMarkDead:
		d.tracker.markDead(ctx, peer)

	case severityMarkBad:
		d.tracker.markBad(ctx, peer, addr)
	}
	return nil
}

type dialErrorSeverity int

const (
	severityDontCare dialErrorSeverity = -iota
	severityMarkBad
	severityMarkDead
)

func classifyDialError(ctx context.Context, peer peer.ID, service *api.ServiceAddress, addr multiaddr.Multiaddr, err error) dialErrorSeverity {
	// User canceled request, don't care
	if errors.Is(err, context.Canceled) {
		return severityDontCare
	}

	// Request timed out, don't care
	if errors.Is(err, context.DeadlineExceeded) {
		return severityDontCare
	}

	// Connection attempted timed out, mark peer bad
	var timeoutError interface{ Timeout() bool }
	if errors.As(err, &timeoutError) && timeoutError.Timeout() {
		slog.InfoCtx(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkBad
	}

	// To many connection attempts failed, mark peer bad
	if errors.Is(err, swarm.ErrDialBackoff) {
		slog.InfoCtx(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkBad
	}

	// Unable to create a connection, mark peer dead
	if errors.Is(err, network.ErrNoConn) {
		slog.InfoCtx(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkDead
	}

	// No known addresses, mark peer dead
	if errors.Is(err, network.ErrNoRemoteAddrs) {
		slog.InfoCtx(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkDead
	}

	// Swarm error, take the worst severity
	var swarmError *swarm.DialError
	if errors.As(err, &swarmError) {
		var s dialErrorSeverity
		for _, err := range swarmError.DialErrors {
			t := classifyDialError(ctx, peer, service, addr, err.Cause)
			if t < s {
				s = t
			}
		}
		return s
	}

	// Unknown error, mark peer bad
	slog.WarnCtx(ctx, "Unknown error while dialing peer", "peer", peer, "service", service, "error", err)
	return severityMarkBad
}
