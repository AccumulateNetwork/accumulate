// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"golang.org/x/exp/slog"
)

type peerTracker interface {
	// markGood marks a peer as good for a given service.
	markGood(ctx context.Context, peer peer.ID, service multiaddr.Multiaddr)

	// markBad marks a peer as bad for a given service.
	markBad(ctx context.Context, peer peer.ID, service multiaddr.Multiaddr)

	// markDead marks a peer as dead.
	markDead(ctx context.Context, peer peer.ID)

	// status returns the status of a peer.
	status(ctx context.Context, peer peer.ID, service multiaddr.Multiaddr) api.KnownPeerStatus

	// nextGood returns the next good peer for a service.
	nextGood(ctx context.Context, service multiaddr.Multiaddr) (peer.ID, bool)

	// nextBad returns the next bad peer for a service.
	nextBad(ctx context.Context, service multiaddr.Multiaddr) (peer.ID, bool)

	// allGood returns all good peers for a service.
	allGood(ctx context.Context, service multiaddr.Multiaddr) []peer.ID

	// allBad returns all bad peers for a service.
	allBad(ctx context.Context, service multiaddr.Multiaddr) []peer.ID
}

type fakeTracker struct{}

func (fakeTracker) markGood(context.Context, peer.ID, multiaddr.Multiaddr)        {}
func (fakeTracker) markBad(context.Context, peer.ID, multiaddr.Multiaddr)         {}
func (fakeTracker) markDead(context.Context, peer.ID)                             {}
func (fakeTracker) nextGood(context.Context, multiaddr.Multiaddr) (peer.ID, bool) { return "", false }
func (fakeTracker) nextBad(context.Context, multiaddr.Multiaddr) (peer.ID, bool)  { return "", false }
func (fakeTracker) allGood(context.Context, multiaddr.Multiaddr) []peer.ID        { return nil }
func (fakeTracker) allBad(context.Context, multiaddr.Multiaddr) []peer.ID         { return nil }

func (fakeTracker) status(context.Context, peer.ID, multiaddr.Multiaddr) api.KnownPeerStatus {
	return 0
}

type simpleTracker struct {
	mu   sync.RWMutex
	good map[string]*peerQueue
	bad  map[string]*peerQueue
}

func (t *simpleTracker) get(service multiaddr.Multiaddr) (good, bad *peerQueue) {
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

func (t *simpleTracker) markGood(ctx context.Context, peer peer.ID, service multiaddr.Multiaddr) {
	good, bad := t.get(service)
	ok1 := good.Add(peer)
	ok2 := bad.Remove(peer)
	if ok1 || ok2 {
		slog.DebugCtx(ctx, "Marked peer good", "peer", peer, "service", service)
	}
}

func (t *simpleTracker) markBad(ctx context.Context, peer peer.ID, service multiaddr.Multiaddr) {
	good, bad := t.get(service)
	ok1 := good.Remove(peer)
	ok2 := bad.Add(peer)
	if ok1 || ok2 {
		slog.DebugCtx(ctx, "Marked peer bad", "peer", peer, "service", service)
	}
}

func (t *simpleTracker) markDead(ctx context.Context, peer peer.ID) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var ok bool
	for _, q := range t.good {
		if q.Remove(peer) {
			ok = true
		}
	}
	for _, q := range t.bad {
		if q.Remove(peer) {
			ok = true
		}
	}
	if ok {
		slog.DebugCtx(ctx, "Marked peer dead", "peer", peer)
	}
}

func (t *simpleTracker) status(ctx context.Context, peer peer.ID, service multiaddr.Multiaddr) api.KnownPeerStatus {
	good, bad := t.get(service)
	switch {
	case good.Has(peer):
		return api.PeerStatusIsKnownGood
	case bad.Has(peer):
		return api.PeerStatusIsKnownBad
	default:
		return 0
	}
}

func (t *simpleTracker) nextGood(ctx context.Context, service multiaddr.Multiaddr) (peer.ID, bool) {
	good, _ := t.get(service)
	return good.Next()
}

func (t *simpleTracker) nextBad(ctx context.Context, service multiaddr.Multiaddr) (peer.ID, bool) {
	_, bad := t.get(service)
	return bad.Next()
}

func (t *simpleTracker) allGood(ctx context.Context, service multiaddr.Multiaddr) []peer.ID {
	good, _ := t.get(service)
	return good.All()
}

func (t *simpleTracker) allBad(ctx context.Context, service multiaddr.Multiaddr) []peer.ID {
	_, bad := t.get(service)
	return bad.All()
}
