// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"fmt"
	"io"
	"sync"
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
	host      dialerHost
	peers     dialerPeers
	tracker   dialerTracker
	goodPeers map[string][]peer.AddrInfo // string is the ServiceAddress, and known good peers
	mutex     *sync.Mutex
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

type dialerTracker interface {
	// isBad returns true if the peer is bad.
	isBad(peer.ID) bool

	// markBad increments the peer's error count. isBad must have been called
	// first with the same peer ID.
	markBad(peer.ID)
}

type fakeTraker struct{}

func (fakeTraker) isBad(peer.ID) bool { return false }
func (fakeTraker) markBad(peer.ID)    {}

type simpleTracker struct {
	sync.RWMutex
	best           int
	peerErrorCount map[peer.ID]int
	countOfCount   []int
}

func (t *simpleTracker) isBad(p peer.ID) bool {
	t.RLock()
	v, ok := t.peerErrorCount[p]
	t.RUnlock()
	if ok {
		return v > t.best
	}

	t.Lock()
	defer t.Unlock()

	if t.peerErrorCount == nil {
		t.peerErrorCount = map[peer.ID]int{}
		t.countOfCount = []int{0}
	}

	v, ok = t.peerErrorCount[p]
	if ok {
		return v > t.best
	}

	t.peerErrorCount[p] = 0
	t.countOfCount[0]++
	if t.best > 0 {
		t.best = 0
	}
	return false
}

func (t *simpleTracker) markBad(p peer.ID) {
	t.Lock()
	defer t.Unlock()

	if t.peerErrorCount == nil {
		t.peerErrorCount = map[peer.ID]int{}
		t.countOfCount = []int{0}
	}

	// Increment the peer's error count
	v := t.peerErrorCount[p]
	t.peerErrorCount[p] = v + 1

	// Decrement the previous count-of-count
	t.countOfCount[v]--

	// Increment the current count-of-count
	if v+1 < len(t.countOfCount) {
		t.countOfCount[v+1]++
	} else {
		t.countOfCount = append(t.countOfCount, 1)
	}

	// Update best
	for t.countOfCount[t.best] <= 0 && t.best < len(t.countOfCount) {
		t.best++
	}
}

// stream is a [message.Stream] with an associated [peerState].
type stream struct {
	peer   peer.ID
	conn   io.ReadWriteCloser
	stream message.Stream
}

// DialNetwork returns a [message.MultiDialer] that opens a stream to a node
// that can provides a given service.
func (n *Node) DialNetwork() message.MultiDialer {
	var t dialerTracker
	if n.trackPeers {
		t = &simpleTracker{}
	} else {
		t = fakeTraker{}
	}
	return dialer{
		host:      n,
		peers:     n.peermgr,
		tracker:   t,
		goodPeers: make(map[string][]peer.AddrInfo),
		mutex:     new(sync.Mutex)}
}

// Dial dials the given address. The address must include an /acc component and
// may include a /p2p component. Dial will return an error if the address
// includes any other components.
//
// If the address is serviceable by the receiving node, the stream will be
// handled locally without requiring any network transport. Otherwise Dial will
// find an appropriate peer that can service the address. If no peer can be
// found, Dial will return [errors.NoPeer].
func (d dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (stream message.Stream, err error) {
	net, peer, sa, err := unpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Retry if fails, as a query to the network might take a bit of time.
	// Right now, the code gives the network two second.
	if peer == "" {
		for i := 0; i < 20; i++ {
			if stream, err = d.newNetworkStream(ctx, sa, net); err == nil {
				return stream, err
			}
			time.Sleep(time.Second / 10)
		}
		return stream, err
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

	var peerID peer.ID
	if cPeer != nil {
		peerID = peer.ID(cPeer.RawValue())
	}

	var net string
	if cNetwork != nil {
		net = string(cNetwork.RawValue())
	}

	return net, peerID, sa, nil
}

// BadDial notifies the dialer that a transport error was encountered while
// processing the stream.
func (d dialer) BadDial(ctx context.Context, addr multiaddr.Multiaddr, s message.Stream, err error) bool {
	d.tracker.markBad(s.(*stream).peer)
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
		p, q := message.DuplexPipe(ctx)
		go s.handler(p)
		return q, nil
	}

	// Open a new stream
	return openStreamFor(ctx, d.host, peer, sa, true)
}

func openStreamFor(ctx context.Context, host dialerHost, peer peer.ID, sa *api.ServiceAddress, close bool) (*stream, error) {
	conn, err := host.getPeerService(ctx, peer, sa)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Close the stream when the context is canceled
	if close {
		go func() { <-ctx.Done(); _ = conn.Close() }()
	}

	s := new(stream)
	s.peer = peer
	s.conn = conn
	s.stream = message.NewStream(conn)
	return s, nil
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
func (d *dialer) newNetworkStream(ctx context.Context, sa *api.ServiceAddress, netName string) (message.Stream, error) {
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Check if we participate in this partition
	service, ok := d.host.getOwnService(netName, sa)
	if ok {
		p, q := message.DuplexPipe(ctx)
		go service.handler(p)
		return q, nil
	}

	// Construct an address for the service
	addr, err := sa.MultiaddrFor(netName)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	sem := make(chan struct{}, 4) // Max number of simultaneous requests
	var s *stream

	// ============= Note the locking in this range ===== vvvv
	d.mutex.Lock()
	pList := d.goodPeers[addr.String()]
	if len(pList) > 4 { // For no peers, or just 1 peer, look to the network
		p := pList[0]
		copy(pList, pList[1:])
		pList[len(pList)-1] = p
		fmt.Println("roll")
		for _, p := range pList {
			d.mutex.Unlock()
			if s = d.attemptDial(ctx, sem, p, sa, addr); s != nil {
				go func() { <-ctx.Done(); _ = s.conn.Close() }()
				return s, nil
			}
			d.mutex.Lock()
		}
		d.goodPeers[addr.String()] = pList // Update our list of peers, even though we shouldn't have to
	}
	d.mutex.Unlock()
	// =============== Every path out this range must unlock! ===== ^^^^

	// Query the DHT for peers that provide the service
	peers, err := d.peers.getPeers(callCtx, addr, 10)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Try to connect to peers, concurrently to reduce the impact of timeouts
	streams := make(chan *stream)
	wg := new(sync.WaitGroup)

outer:
	for s == nil {
		select {
		case p, ok := <-peers:
			if !ok {
				break outer
			}

			// Got a new peer
			wg.Add(1)
			go d.wgAttemptDial(ctx, sem, wg, p, sa, addr, streams)

		case s = <-streams:
			// Got a live connection
			break outer
		}
	}

	// Close any extra streams
	go func() { wg.Wait(); close(streams) }()
	for c := range streams {
		if s == nil {
			s = c
			continue
		}

		if c != nil {
			err = c.conn.Close()
			if err != nil {
				slog.ErrorCtx(ctx, "Error while closing extra stream", "error", err)
			}
		}
	}

	if s == nil {
		return nil, errors.NoPeer.WithFormat("no live peers for %v", sa)
	}

	// Close the connection when the context is canceled
	go func() {
		<-ctx.Done()
		_ = s.conn.Close()
	}()

	return s, nil
}

func (d *dialer) wgAttemptDial(
	ctx context.Context,
	sem chan struct{},
	wg *sync.WaitGroup,
	p peer.AddrInfo,
	sa *api.ServiceAddress,
	addr multiaddr.Multiaddr,
	conn chan<- *stream) {

	defer wg.Done()
	s := d.attemptDial(ctx, sem, p, sa, addr)
	conn <- s
}

func (d *dialer) attemptDial(
	ctx context.Context,
	sem chan struct{},
	p peer.AddrInfo,
	sa *api.ServiceAddress,
	addr multiaddr.Multiaddr) *stream {

	if d.tracker.isBad(p.ID) {
		return nil
	}

	sem <- struct{}{}
	defer func() { <-sem }()

	// Open a stream
	slog.DebugCtx(ctx, "Dialing peer", "peer", p.ID, "service", sa)
	s, err := openStreamFor(ctx, d.host, p.ID, sa, false)

	d.mutex.Lock()
	defer d.mutex.Unlock()
	peers := d.goodPeers[addr.String()]

	if err == nil {
		slog.DebugCtx(ctx, "Successfully dialed peer", "peer", p.ID, "service", sa)
		for _, kp := range peers {
			if kp.ID == p.ID {
				return s
			}
		}
		peers = append(peers, p)
		ma := addr.String()
		d.goodPeers[ma] = peers
		return s
	}

	for i, kp := range peers {
		if kp.ID == p.ID {
			copy(peers[:i], peers[i+1:]) // Delete the failing peer;
			peers = peers[:len(peers)-1]
			d.goodPeers[addr.String()] = peers
			break
		}
	}

	d.tracker.markBad(p.ID)

	var timeoutError interface{ Timeout() bool }
	switch {
	case errors.Is(err, network.ErrNoConn),
		errors.Is(err, network.ErrNoRemoteAddrs),
		errors.Is(err, swarm.ErrDialBackoff),
		errors.As(err, &timeoutError) && timeoutError.Timeout():
		// Ignore and try again
		go slog.DebugCtx(ctx, "Unable to dial peer", "peer", p.ID, "service", sa, "error", err)

	default:
		// Log and try again
		go slog.WarnCtx(ctx, "Unknown error while dialing peer", "peer", p.ID, "service", sa, "error", err)
	}
	return nil
}

// selfDialer always dials the [Node] directly.
type selfDialer Node

// DialSelf returns a [message.Dialer] that always returns a stream for the current node.
func (n *Node) DialSelf() message.Dialer { return (*selfDialer)(n) }

// Dial returns a stream for the current node.
func (d *selfDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	// Parse the address
	_, peer, sa, err := unpackAddress(addr)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if peer != "" && peer != d.host.ID() {
		s, err := openStreamFor(ctx, (*Node)(d), peer, sa, true)
		return s, errors.UnknownError.Wrap(err)
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
