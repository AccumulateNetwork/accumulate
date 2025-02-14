// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"log/slog"
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
)

type Discoverer interface {
	Discover(context.Context, *DiscoveryRequest) (DiscoveryResponse, error)
}

type DiscoveryRequest struct {
	Network string
	Service *api.ServiceAddress
	Limit   int
	Timeout time.Duration
}

type DiscoveryResponse interface {
	isDiscoveryResponse()
}

type DiscoveredPeers <-chan peer.AddrInfo
type DiscoveredLocal func(context.Context) (message.Stream, error)

func (DiscoveredPeers) isDiscoveryResponse() {}
func (DiscoveredLocal) isDiscoveryResponse() {}

type Tracker interface {
	Mark(peer peer.ID, service multiaddr.Multiaddr, status api.KnownPeerStatus)
	Status(peer peer.ID, service multiaddr.Multiaddr) api.KnownPeerStatus
	Next(service multiaddr.Multiaddr, status api.KnownPeerStatus) (peer.ID, bool)
	All(service multiaddr.Multiaddr, status api.KnownPeerStatus) []peer.ID
}

// dialer implements [message.MultiDialer].
type dialer struct {
	host    Connector
	peers   Discoverer
	tracker Tracker
	lastTry sync.Map
}

var _ message.MultiDialer = (*dialer)(nil)

// Dial dials the given address. The address must include an /acc component and
// may include a /p2p component. Dial will return an error if the address
// includes any other components.
//
// If the address is serviceable by the receiving node, the stream will be
// handled locally without requiring any network transport. Otherwise Dial will
// find an appropriate peer that can service the address. If no peer can be
// found, Dial will return [errors.NoPeer].
func (d *dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (stream message.Stream, err error) {
	net, peer, sa, ip, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if sa == nil {
		return nil, errors.BadRequest.WithFormat("invalid address %v", addr)
	}

	if ip != nil && peer == "" {
		return nil, errors.BadRequest.WithFormat("cannot specify address without peer ID")
	}

	if peer == "" {
		// Do not set the wait group
		return d.newNetworkStream(ctx, sa, net, nil)
	}

	// Open a new stream
	return openStreamFor(ctx, d.host, &ConnectionRequest{
		Service:  sa,
		PeerID:   peer,
		PeerAddr: ip,
	})
}

// BadDial notifies the dialer that a transport error was encountered while
// processing the stream.
func (d *dialer) BadDial(ctx context.Context, addr multiaddr.Multiaddr, s message.Stream, err error) bool {
	ss, ok := s.(*stream)
	if !ok {
		slog.InfoContext(ctx, "Bad dial", "address", addr, "error", err)
		return false
	}

	slog.InfoContext(ctx, "Bad dial", "peer", ss.peer, "address", addr, "error", err)

	if errors.Is(err, errors.EncodingError) /*|| errors.Is(err, errors.StreamAborted) */ {
		// Don't mark a peer bad if there's an encoding failure. Is this a good
		// idea?
		return false
	}

	d.tracker.Mark(ss.peer, addr, api.PeerStatusIsKnownBad)
	return true
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
	// Construct an address for the service
	addr, err := service.MultiaddrFor(netName)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Discover peers that provide the service
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := d.peers.Discover(callCtx, &DiscoveryRequest{
		Network: netName,
		Service: service,
		Limit:   10,
		Timeout: 1 * time.Second,
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	var peers <-chan peer.AddrInfo
	switch resp := resp.(type) {
	case DiscoveredLocal:
		return resp(ctx)
	case DiscoveredPeers:
		peers = resp
	default:
		panic("invalid discovery response")
	}

	// Check the remaining unknown peers from the DHT (non-blocking)
	defer func() {
		for i := 0; i < 10; {
			select {
			case peer, ok := <-peers:
				if !ok {
					return
				}

				if d.tracker.Status(peer.ID, addr) != api.PeerStatusIsUnknown {
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
	if len(d.tracker.All(addr, api.PeerStatusIsKnownGood)) >= 4 {
		s := d.dialFromTracker(ctx, service, addr, wg)
		if s != nil {
			return s, nil
		}
	}

	// Try peers from the DHT
	var bad []peer.ID
	for peer := range peers {
		// Skip known-bad peers
		if d.tracker.Status(peer.ID, addr) == api.PeerStatusIsKnownBad {
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
	if bad, ok := d.tracker.Next(addr, api.PeerStatusIsKnownBad); ok {
		d.tryDial(bad, service, addr, wg)
	}

	// Make 10 attempts to find a known-good peer, but only if there are at
	// least 4 known-good peers
	var first peer.ID
	for i := 0; i < 10; i++ {
		// Get the next
		peer, ok := d.tracker.Next(addr, api.PeerStatusIsKnownGood)
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
	if didLoad && time.Since(*last.time.Load()) < backoffTime(last.count.Load()) {
		// The last attempt was less than a minute ago
		return

	} else {
		// Update the attempt time and count
		t := time.Now()
		last.count.Add(1)
		last.time.Store(&t)
	}

	go func() {
		// Panic protection
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panicked while handling stream", "error", r, "stack", debug.Stack(), "module", "api")
			}
		}()

		if wg != nil {
			defer wg.Done()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		d.dial(ctx, peer, service, addr)
	}()
}

// backoffTime returns the smaller of 2ⁿ minutes  or 1 day.
func backoffTime(attempts int32) time.Duration {
	d := time.Minute << time.Duration(attempts)
	if d > 24*time.Hour {
		d = 24 * time.Hour
	}
	return d
}

func (d *dialer) dial(ctx context.Context, peer peer.ID, service *api.ServiceAddress, addr multiaddr.Multiaddr) *stream {
	// Panic protection
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panicked while dialing a peer", "error", r, "stack", debug.Stack(), "module", "api")
		}
	}()

	// Open a stream
	stream, err := openStreamFor(ctx, d.host, &ConnectionRequest{
		Service: service,
		PeerID:  peer,
	})
	if err == nil {
		// Mark the peer good
		d.tracker.Mark(peer, addr, api.PeerStatusIsKnownGood)
		return stream
	}

	// Log the error and mark the peer
	switch classifyDialError(ctx, peer, service, addr, err) {
	case severityMarkDead:
		d.tracker.Mark(peer, addr, api.PeerStatusIsUnknown)

	case severityMarkBad:
		d.tracker.Mark(peer, addr, api.PeerStatusIsKnownBad)
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
		slog.InfoContext(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkBad
	}

	// To many connection attempts failed, mark peer bad
	if errors.Is(err, swarm.ErrDialBackoff) {
		slog.InfoContext(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkBad
	}

	// Unable to create a connection, mark peer dead
	if errors.Is(err, network.ErrNoConn) {
		slog.InfoContext(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkDead
	}

	// No known addresses, mark peer dead
	if errors.Is(err, network.ErrNoRemoteAddrs) ||
		errors.Is(err, swarm.ErrNoAddresses) {
		slog.InfoContext(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
		return severityMarkDead
	}

	// Failed to dial, mark peer bad
	if errors.Is(err, swarm.ErrDialBackoff) ||
		errors.Is(err, swarm.ErrNoGoodAddresses) {
		slog.InfoContext(ctx, "Unable to dial peer", "peer", peer, "service", service, "error", err)
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
	slog.WarnContext(ctx, "Unknown error while dialing peer", "peer", peer, "service", service, "error", err)
	return severityMarkBad
}
