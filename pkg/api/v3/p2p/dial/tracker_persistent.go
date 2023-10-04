// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/peerdb"
	"golang.org/x/exp/slog"
)

type PersistentTracker struct {
	context context.Context
	cancel  context.CancelFunc
	db      *peerdb.DB
	file    string
	host    Connector
	peers   Discoverer
	stopwg  *sync.WaitGroup
}

type PersistentTrackerOptions struct {
	Filename         string
	Host             Connector
	Peers            Discoverer
	PersistFrequency time.Duration
}

func NewPersistentTracker(ctx context.Context, opts PersistentTrackerOptions) (*PersistentTracker, error) {
	t := new(PersistentTracker)
	t.db = peerdb.New()
	t.file = opts.Filename
	t.host = opts.Host
	t.peers = opts.Peers
	t.stopwg = new(sync.WaitGroup)

	t.context, t.cancel = context.WithCancel(ctx)

	// Ensure the file can be created
	f, err := os.OpenFile(opts.Filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// If the file is non-empty, read it
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > 0 {
		err = t.db.Load(f)
		if err != nil {
			return nil, err
		}
	}

	if opts.PersistFrequency == 0 {
		opts.PersistFrequency = time.Hour
	}
	t.stopwg.Add(1)
	go t.writeDb(opts.PersistFrequency)

	return t, nil
}

func (t *PersistentTracker) Stop() {
	t.cancel()
	t.stopwg.Wait()
}

func (t *PersistentTracker) writeDb(frequency time.Duration) {
	defer t.stopwg.Done()

	tick := time.NewTicker(frequency)
	go func() { <-t.context.Done(); tick.Stop() }()

	for range tick.C {
		slog.InfoCtx(t.context, "Writing peer database")

		f, err := os.OpenFile(t.file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			slog.ErrorCtx(t.context, "Failed to open peer database", "error", err)
			continue
		}

		err = t.db.Store(f)
		if err != nil {
			slog.ErrorCtx(t.context, "Failed to write peer database", "error", err)
		}

		f.Close()
	}
}

func (t *PersistentTracker) Mark(peer peer.ID, addr multiaddr.Multiaddr, status api.KnownPeerStatus) {
	netName, _, service, inetAddr, err := api.UnpackAddress(addr)
	if err != nil {
		panic(err)
	}

	switch status {
	case api.PeerStatusIsKnownGood:
		if inetAddr != nil {
			// Mark that we connected to the address
			t.db.Peer(peer).Address(inetAddr).Last.DidSucceed()
		}

		// Mark that we connected to the service
		t.db.Peer(peer).Network(netName).Service(service).Last.DidSucceed()

	case api.PeerStatusIsUnknown:
		// Reset the last connected time
		t.db.Peer(peer).Network(netName).Service(service).Last.Success = nil

	case api.PeerStatusIsKnownBad:
		// Don't do anything - last attempt will be greater than last success
	}
}

func (t *PersistentTracker) Status(peer peer.ID, addr multiaddr.Multiaddr) api.KnownPeerStatus {
	netName, _, service, _, err := api.UnpackAddress(addr)
	if err != nil {
		panic(err)
	}

	s := t.db.Peer(peer).Network(netName).Service(service)
	return statusForLast(s.Last)
}

func statusForLast(l peerdb.LastStatus) api.KnownPeerStatus {
	switch {
	case l.Attempt == nil:
		// No connection attempted
		return api.PeerStatusIsUnknown

	case l.Success == nil:
		// No successful connection

		// Attempt was too long ago?
		if attemptIsTooOld(l) {
			return api.PeerStatusIsKnownBad
		}

		// Attempt was recent
		return api.PeerStatusIsUnknown

	case l.Attempt.After(*l.Success):
		// Connection attempted since the last success

		// Attempt was too long ago?
		if attemptIsTooOld(l) {
			return api.PeerStatusIsKnownBad
		}

		// Last success was too long ago?
		return statusForLastSuccess(l)

	default:
		// Last attempt was successful

		// Was it too long ago?
		return statusForLastSuccess(l)
	}
}

func attemptIsTooOld(l peerdb.LastStatus) bool {
	return time.Since(*l.Attempt) > time.Second
}

func statusForLastSuccess(l peerdb.LastStatus) api.KnownPeerStatus {
	// Last success was too long ago?
	if time.Since(*l.Success) > 10*time.Minute {
		return api.PeerStatusIsUnknown
	}

	// Last success was recent
	return api.PeerStatusIsKnownGood
}

func (t *PersistentTracker) Next(addr multiaddr.Multiaddr, status api.KnownPeerStatus) (peer.ID, bool) {
	netName, _, service, _, err := api.UnpackAddress(addr)
	if err != nil {
		panic(err)
	}

	// Get all the candidates with the given status
	var candidates []*peerdb.PeerStatus
	for _, p := range t.db.Peers() {
		s := p.Network(netName).Service(service)
		if statusForLast(s.Last) == status {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}

	switch status {
	case api.PeerStatusIsKnownGood,
		api.PeerStatusIsKnownBad:
		// Pick the least recently used one
		sort.Slice(candidates, func(i, j int) bool {
			a := candidates[i].Network(netName).Service(service)
			b := candidates[j].Network(netName).Service(service)
			switch {
			case a.Last.Attempt == nil || b.Last.Attempt == nil:
				return false
			case a.Last.Attempt == nil:
				return true
			case b.Last.Attempt == nil:
				return false
			default:
				return a.Last.Attempt.Before(*b.Last.Attempt)
			}
		})
	}

	candidates[0].Network(netName).Service(service).Last.DidAttempt()
	return candidates[0].ID, true
}

func (t *PersistentTracker) All(addr multiaddr.Multiaddr, status api.KnownPeerStatus) []peer.ID {
	netName, _, service, _, err := api.UnpackAddress(addr)
	if err != nil {
		panic(err)
	}

	var peers []peer.ID
	for _, p := range t.db.Peers() {
		s := p.Network(netName).Service(service)
		if statusForLast(s.Last) == status {
			peers = append(peers, p.ID)
		}
	}
	return peers
}
