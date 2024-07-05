// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"log/slog"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

type SimpleTracker struct {
	mu   sync.RWMutex
	good map[string]*peerQueue
	bad  map[string]*peerQueue
}

var _ Tracker = (*SimpleTracker)(nil)

func (t *SimpleTracker) get(service multiaddr.Multiaddr) (good, bad *peerQueue) {
	key := service.String()
	t.mu.RLock()
	good, ok := t.good[key]
	bad = t.bad[key]
	t.mu.RUnlock()
	if ok {
		return good, bad
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	good, ok = t.good[key]
	bad = t.bad[key]
	if ok {
		return good, bad
	}

	if t.good == nil {
		t.good = map[string]*peerQueue{}
		t.bad = map[string]*peerQueue{}
	}

	good = new(peerQueue)
	bad = new(peerQueue)
	t.good[key] = good
	t.bad[key] = bad
	return good, bad
}

func (t *SimpleTracker) Mark(peer peer.ID, service multiaddr.Multiaddr, status api.KnownPeerStatus) {
	good, bad := t.get(service)
	switch status {
	case api.PeerStatusIsKnownGood:
		ok1 := good.Add(peer)
		ok2 := bad.Remove(peer)
		if ok1 || ok2 {
			slog.Debug("Marked peer good", "peer", peer, "service", service)
		}

	case api.PeerStatusIsKnownBad:
		ok1 := good.Remove(peer)
		ok2 := bad.Add(peer)
		if ok1 || ok2 {
			slog.Debug("Marked peer bad", "peer", peer, "service", service)
		}

	case api.PeerStatusIsUnknown:
		ok1 := good.Remove(peer)
		ok2 := bad.Remove(peer)
		if ok1 || ok2 {
			slog.Debug("Marked peer dead", "peer", peer, "service", service)
		}
	}
}

func (t *SimpleTracker) Status(peer peer.ID, service multiaddr.Multiaddr) api.KnownPeerStatus {
	good, bad := t.get(service)
	switch {
	case good.Has(peer):
		return api.PeerStatusIsKnownGood
	case bad.Has(peer):
		return api.PeerStatusIsKnownBad
	default:
		return api.PeerStatusIsUnknown
	}
}

func (t *SimpleTracker) Next(service multiaddr.Multiaddr, status api.KnownPeerStatus) (peer.ID, bool) {
	good, bad := t.get(service)
	switch status {
	case api.PeerStatusIsKnownGood:
		return good.Next()
	case api.PeerStatusIsKnownBad:
		return bad.Next()
	default:
		return "", false
	}
}

func (t *SimpleTracker) All(service multiaddr.Multiaddr, status api.KnownPeerStatus) []peer.ID {
	good, bad := t.get(service)
	switch status {
	case api.PeerStatusIsKnownGood:
		return good.All()
	case api.PeerStatusIsKnownBad:
		return bad.All()
	default:
		return nil
	}
}
