// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/record"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
)

// rpcProtocolPrefix is what every Accumulate service's protocol ID starts
// with (idRpc).
const rpcProtocolPrefix = "/acc/rpc/"

// serviceMemory is the host's peerstore, keeping a connected peer's services
// as the peer announced them (#4610).
//
// A node learns which services a connected peer serves from libp2p identify:
// the answer to the identify it sends when the connection forms, and the push
// the peer sends each time it registers a handler. Neither carries a sequence
// number, the host consumes them in whatever order their stream handlers are
// scheduled, and each one replaces the peer's whole protocol set in the
// peerstore. Under load the push that carried a service is consumed first and
// an older message -- the connect-time answer, or the push of the service
// registered before it -- lands last and takes the service away again. Four
// nodes on two loaded cores formed their mesh with a peer's submit service
// missing from the node it bootstrapped from in 8 of 40 runs, and nothing
// brought it back: identify never speaks of that peer again until the
// connection is remade (MR !1274, "the API mesh never formed" in
// TestASubmissionNeedingASectionNotLoadedIsRedirected).
//
// Nothing withdraws a service while a node runs -- there is no unregister --
// so a service a peer announced on this connection is a service it still
// has. The peerstore keeps every Accumulate service each peer has announced
// and writes it back into every later set of the peer's protocols, so the
// message consumed last cannot drop it. It is done in the write, not after
// it: an event after the fact runs when the next stale write may already
// have landed. A peer is forgotten when its last connection closes: it may
// come back with fewer.
type serviceMemory struct {
	peerstore.Peerstore
	mu    sync.Mutex
	known map[peer.ID]map[protocol.ID]struct{}
}

var _ peerstore.CertifiedAddrBook = (*serviceMemory)(nil)

func newServiceMemory() (*serviceMemory, error) {
	ps, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, err
	}
	return &serviceMemory{Peerstore: ps, known: map[peer.ID]map[protocol.ID]struct{}{}}, nil
}

// forgetOnDisconnect forgets a peer's services when its last connection to
// the host closes.
func (m *serviceMemory) forgetOnDisconnect(h host.Host) {
	h.Network().Notify(&network.NotifyBundle{DisconnectedF: func(_ network.Network, c network.Conn) {
		if len(h.Network().ConnsToPeer(c.RemotePeer())) == 0 {
			m.forget(c.RemotePeer())
		}
	}})
}

// SetProtocols replaces the peer's protocols with the given ones and the
// Accumulate services the peer announced before.
func (m *serviceMemory) SetProtocols(p peer.ID, ids ...protocol.ID) error {
	return m.Peerstore.SetProtocols(p, m.withKnown(p, ids)...)
}

func (m *serviceMemory) AddProtocols(p peer.ID, ids ...protocol.ID) error {
	m.withKnown(p, ids)
	return m.Peerstore.AddProtocols(p, ids...)
}

func (m *serviceMemory) RemoveProtocols(p peer.ID, ids ...protocol.ID) error {
	m.mu.Lock()
	for _, id := range ids {
		delete(m.known[p], id)
	}
	m.mu.Unlock()
	return m.Peerstore.RemoveProtocols(p, ids...)
}

func (m *serviceMemory) RemovePeer(p peer.ID) {
	m.forget(p)
	m.Peerstore.RemovePeer(p)
}

// withKnown learns the Accumulate services among ids and returns ids with
// every service the peer announced before.
func (m *serviceMemory) withKnown(p peer.ID, ids []protocol.ID) []protocol.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	known := m.known[p]
	for _, id := range ids {
		if !strings.HasPrefix(string(id), rpcProtocolPrefix) {
			continue
		}
		if known == nil {
			known = map[protocol.ID]struct{}{}
			m.known[p] = known
		}
		known[id] = struct{}{}
	}
	if len(known) == 0 {
		return ids
	}
	have := map[protocol.ID]struct{}{}
	for _, id := range ids {
		have[id] = struct{}{}
	}
	for id := range known {
		if _, ok := have[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *serviceMemory) forget(p peer.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.known, p)
}

// services returns the Accumulate services the peer announced.
func (m *serviceMemory) services(p peer.ID) []protocol.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []protocol.ID
	for id := range m.known[p] {
		out = append(out, id)
	}
	return out
}

// The host looks for a certified address book in its peerstore by type,
// which embedding the interface does not give a struct.

func (m *serviceMemory) ConsumePeerRecord(s *record.Envelope, ttl time.Duration) (bool, error) {
	return m.Peerstore.(peerstore.CertifiedAddrBook).ConsumePeerRecord(s, ttl)
}

func (m *serviceMemory) GetPeerRecord(p peer.ID) *record.Envelope {
	return m.Peerstore.(peerstore.CertifiedAddrBook).GetPeerRecord(p)
}
