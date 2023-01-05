// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// idWhoami is the [protocol.ID] for Accumulate's peer discovery protocol.
const idWhoami = "/acc/whoami/0.1.0"

// peerManager manages the peer list and peer discovery for a [Node].
type peerManager struct {
	logger      logging.OptionalLogger
	host        host.Host
	mu          *sync.RWMutex
	peers       map[peer.ID]*peerState
	pool        *sync.Map
	getServices func() []*service
	waiter      *sync.Cond
}

// peerState is the state of a peer.
type peerState struct {
	info     *Info
	mu       sync.Mutex
	priority int
}

// newPeerManager constructs a new [peerManager] for the given host with the
// given options.
func newPeerManager(host host.Host, getServices func() []*service, opts Options) (*peerManager, error) {
	// Setup the basics
	m := new(peerManager)
	m.host = host
	m.logger.Set(opts.Logger, "module", "acc-p2p")
	m.mu = new(sync.RWMutex)
	m.pool = new(sync.Map)
	m.peers = map[peer.ID]*peerState{}
	m.getServices = getServices
	m.waiter = sync.NewCond(m.mu.RLocker())

	// Register the peer discovery stream handler
	host.SetStreamHandler(idWhoami, func(s network.Stream) {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("Panic while handling stream", "error", r)
			}
		}()
		defer s.Close()
		m.handleStream(message.NewStreamOf[Whoami](s), s.Conn().RemotePeer())
	})

	// Convert bootstrap peers to AddrInfos
	var addrs []*peer.AddrInfo
	for _, addr := range opts.BootstrapPeers {
		p, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid bootstrap peer address: %w", err)
		}
		addrs = append(addrs, p)
	}

	// Connect to all the bootstrap peers. Keep trying once a second until every
	// bootstrap peer has been found.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("Panic while connecting to bootstrap peers", "error", r)
			}
		}()
		for i := 0; len(addrs) > 0; {
			if m.connectTo(addrs[i]) != nil {
				i++
				time.Sleep(time.Second)
			} else {
				sortutil.RemoveAt(&addrs, i)
			}
			if i >= len(addrs) {
				i = 0
			}
		}
	}()

	return m, nil
}

// addKnown adds a peer to the list of known peers. addKnown returns true if the
// peer was not previously known.
func (m *peerManager) addKnown(peer *Info) bool {
	m.waiter.Broadcast()

	m.mu.RLock()
	p, ok := m.peers[peer.ID]
	m.mu.RUnlock()
	if ok {
		p.info = peer
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok = m.peers[peer.ID]
	if ok {
		p.info = peer
		return false
	}

	p = new(peerState)
	p.info = peer
	m.peers[peer.ID] = p
	return true
}

// getPeer gets the state of a peer by ID.
func (m *peerManager) getPeer(id peer.ID) (*peerState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.peers[id]
	return state, ok
}

// getPeers gets the state of peers by partition.
func (m *peerManager) getPeers(service *api.ServiceAddress) []*peerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	peers := make([]*peerState, 0, len(m.peers))
	for _, p := range m.peers {
		if service == nil || p.info.HasService(service) {
			peers = append(peers, p)
		}
	}
	return peers
}

// maxPriority is the maximum absolute priority allowed before normalization is
// triggered.
const maxPriority = 1 << 25

// adjustPriority adjusts the priority of the given peer. If the peer's priority
// exceeds maxPriority (negative or positive), adjustPriority normalizes the
// priority of all the peers. adjustPriority does this by finding the minimum
// priority of all peers and subtracting that amount from the priority of every
// peer.
//
// This algorithm may perform badly if priorities are incremented. But
// currently, priorities are only ever decremented.
func (m *peerManager) adjustPriority(peer *peerState, delta int) {
	peer.mu.Lock()
	peer.priority += delta
	pri := peer.priority
	peer.mu.Unlock()

	if maxPriority > pri && pri > -maxPriority {
		return
	}

	// Reset priorities
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find the minimum priority
	var minPri int
	var set bool
	for _, p := range m.peers {
		p.mu.Lock()
		defer p.mu.Unlock()

		if !set || p.priority < minPri {
			minPri = p.priority
		}
		set = true
	}

	// Subtract the minimum from every peer's priority
	for _, p := range m.peers {
		p.priority -= minPri
	}
}

// whoami builds a Whoami message for this node.
func (m *peerManager) whoami(remote peer.ID) *Whoami {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w := new(Whoami)
	w.Self = &Info{ID: m.host.ID()}
	for _, s := range m.getServices() {
		w.Self.Services = append(w.Self.Services, s.info())
	}
	for _, peer := range m.peers {
		if peer.info.ID == remote {
			continue
		}
		ai := new(AddrInfo)
		ai.Info = *peer.info
		ai.Addrs = m.host.Peerstore().Addrs(peer.info.ID)
		w.Known = append(w.Known, ai)
	}
	return w
}

// handleStream handles a peer discovery stream.
func (m *peerManager) handleStream(s message.StreamOf[*Whoami], remotePeer peer.ID) {
	// Build a Whoami message for this node
	w := m.whoami(remotePeer)

	// Send Whoami to the peer
	err := s.Write(w)
	if err != nil {
		m.logger.Error("Failed to write message to peer, rejecting", "error", err, "peer", remotePeer)
		return
	}

	// Read Whoami from the peer
	w, err = s.Read()
	if err != nil {
		m.logger.Error("Failed to read message from peer, rejecting", "error", err, "peer", remotePeer)
		return
	}

	// Add the peer to the known list
	m.addKnown(w.Self)

	// For each other peer the peer knows of
	for _, p := range w.Known {
		// See docs/developer/rangevarref.md
		p := p

		// Add the addresses it knows to our peer store
		m.host.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.AddressTTL)

		// There's nothing else to do if we already know the peer or the sender
		// didn't include any addresses
		if !m.addKnown(&p.Info) || len(p.Addrs) == 0 {
			continue
		}

		// Connect to the peer to exchange Whoami messages
		go func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("Panic while connecting to peer", "error", r, "peer", p)
				}
			}()

			err = m.connectTo(&peer.AddrInfo{
				ID:    p.ID,
				Addrs: p.Addrs,
			})
			if err == nil {
				return
			}
			m.logger.Error("All addresses provided for peer are invalid", "from", remotePeer, "peer", p)
		}()
	}
}

// connectTo connects to a peer
func (m *peerManager) connectTo(p *peer.AddrInfo) error {
	// Connect
	err := m.host.Connect(context.Background(), *p)
	if err != nil {
		return errors.UnknownError.WithFormat("connect: %w", err)
	}

	// Open a stream
	s, err := m.host.NewStream(context.Background(), p.ID, idWhoami)
	if err != nil {
		return errors.UnknownError.WithFormat("open stream: %w", err)
	}
	defer s.Close()

	// Execute peer discovery
	m.handleStream(message.NewStreamOf[Whoami](s), s.Conn().RemotePeer())
	return nil
}

// advertizeNewService advertizes new whoami info to everyone.
func (m *peerManager) advertizeNewService() error {
	m.waiter.Broadcast()

	for _, p := range m.getPeers(nil) {
		// Open a stream
		s, err := m.host.NewStream(context.Background(), p.info.ID, idWhoami)
		if err != nil {
			return errors.UnknownError.WithFormat("open stream: %w", err)
		}
		defer s.Close()

		// Execute peer discovery
		m.handleStream(message.NewStreamOf[Whoami](s), s.Conn().RemotePeer())
	}
	return nil
}

// waitFor blocks until the node has a peer that provides the given address.
func (m *peerManager) waitFor(sa *api.ServiceAddress) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for {
		// Do we have the service?
		_, ok := sortutil.Search(m.getServices(), func(s *service) int { return s.address.Compare(sa) })
		if ok {
			return
		}

		// Does a peer have the service?
		for _, p := range m.peers {
			if p.info.HasService(sa) {
				return
			}
		}

		// Wait
		m.waiter.Wait()
	}
}
