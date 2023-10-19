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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/peerdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type PersistentTracker struct {
	context context.Context
	cancel  context.CancelFunc
	db      *peerdb.DB
	file    string
	network string
	host    Connector
	peers   Discoverer
	stopwg  *sync.WaitGroup

	successThreshold time.Duration
}

type PersistentTrackerOptions struct {
	Network          string
	Filename         string
	Host             Connector
	Peers            Discoverer
	PersistFrequency time.Duration
	ScanFrequency    time.Duration
}

const defaultPersistFrequency = time.Hour
const defaultScanFrequency = time.Hour

func NewPersistentTracker(ctx context.Context, opts PersistentTrackerOptions) (*PersistentTracker, error) {
	t := new(PersistentTracker)
	t.db = peerdb.New()
	t.file = opts.Filename
	t.network = opts.Network
	t.host = opts.Host
	t.peers = opts.Peers
	t.stopwg = new(sync.WaitGroup)
	t.context, t.cancel = context.WithCancel(ctx)

	// Set the success threshold ~5% higher than the scan frequency
	if opts.ScanFrequency == 0 {
		t.successThreshold = defaultScanFrequency
	} else {
		t.successThreshold = opts.ScanFrequency
	}
	t.successThreshold = 17 * t.successThreshold / 16

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

	// Launch async jobs
	t.runJob(t.writeDb, opts.PersistFrequency, defaultPersistFrequency, false)

	if opts.Network == "" {
		slog.Info("Scanning disabled, network unspecified")
	} else {
		t.runJob(t.scanPeers, opts.ScanFrequency, defaultScanFrequency, true)
	}

	return t, nil
}

func (t *PersistentTracker) Stop() {
	t.cancel()
	t.stopwg.Wait()
}

func (t *PersistentTracker) runJob(fn func(time.Duration), frequency, defaultFrequency time.Duration, immediate bool) {
	if frequency == 0 {
		frequency = defaultFrequency
	}

	t.stopwg.Add(1)

	go func() {
		defer t.stopwg.Done()

		if immediate {
			fn(frequency)
		}

		tick := time.NewTicker(frequency)
		go func() { <-t.context.Done(); tick.Stop() }()

		for range tick.C {
			fn(frequency)
		}
	}()
}

func (t *PersistentTracker) writeDb(time.Duration) {
	slog.InfoCtx(t.context, "Writing peer database")

	f, err := os.OpenFile(t.file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		slog.ErrorCtx(t.context, "Failed to open peer database", "error", err)
		return
	}
	defer f.Close()

	err = t.db.Store(f)
	if err != nil {
		slog.ErrorCtx(t.context, "Failed to write peer database", "error", err)
	}
}

func (t *PersistentTracker) scanPeers(duration time.Duration) {
	slog.InfoCtx(t.context, "Scanning for peers")

	// Run discovery
	resp, err := t.peers.Discover(t.context, &DiscoveryRequest{
		Timeout: duration,
		Network: t.network,
	})
	if err != nil {
		slog.ErrorCtx(t.context, "Failed to scan for peers", "error", err)
		return
	}

	// The response should only ever be DiscoveredPeers
	peers, ok := resp.(DiscoveredPeers)
	if !ok {
		slog.ErrorCtx(t.context, "Failed to scan for peers", "error", errors.InternalError.WithFormat("bad discovery response: want %T, got %T", make(DiscoveredPeers), resp))
		return
	}

	// Scan each peer
	ctx, cancel := context.WithTimeout(t.context, duration/2)
	defer cancel()

	wg := new(sync.WaitGroup)
	for peer := range peers {
		wg.Add(1)
		peer := peer
		go func() {
			defer wg.Done()
			t.scanPeer(ctx, peer)
		}()
	}

	wg.Wait()
}

func (t *PersistentTracker) scanPeer(ctx context.Context, peer peer.AddrInfo) {
	slog.InfoCtx(t.context, "Scanning peer", "id", peer.ID)

	// TODO Check addresses

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	creq := &ConnectionRequest{
		Service: api.ServiceTypeNode.Address(),
		PeerID:  peer.ID,
	}
	if len(peer.Addrs) > 0 {
		creq.PeerAddr = peer.Addrs[0]
	}

	t.db.Peer(peer.ID).Network(t.network).Service(creq.Service).Last.DidAttempt()
	s, err := t.host.Connect(ctx, creq)
	if err != nil {
		slog.Info("Unable to connect to peer", "peer", peer.ID, "error", err)
		return
	}
	t.db.Peer(peer.ID).Network(t.network).Service(creq.Service).Last.DidSucceed()

	err = s.Write(&message.NodeInfoRequest{})
	if err != nil {
		slog.Info("Failed to request node info", "peer", peer.ID, "error", err)
		return
	}
	res, err := s.Read()
	if err != nil {
		slog.Info("Failed to request node info", "peer", peer.ID, "error", err)
		return
	}
	var ni *message.NodeInfoResponse
	switch res := res.(type) {
	case *message.ErrorResponse:
		slog.Info("Failed to request node info", "peer", peer.ID, "error", res.Error)
		return
	case *message.NodeInfoResponse:
		ni = res
	default:
		slog.Info("Invalid node info response", "peer", peer.ID, "want", message.TypeNodeInfoResponse, "got", res.Type())
		return
	}

	for _, svc := range ni.Value.Services {
		slog.InfoCtx(t.context, "Attempting to conenct to service", "id", peer.ID, "service", svc)

		t.db.Peer(peer.ID).Network(t.network).Service(svc).Last.DidAttempt()
		_, err := t.host.Connect(ctx, &ConnectionRequest{Service: svc, PeerID: peer.ID})
		if err != nil {
			slog.Info("Unable to connect to peer", "peer", peer.ID, "error", err)
			return
		}
		t.db.Peer(peer.ID).Network(t.network).Service(svc).Last.DidSucceed()
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
	return t.statusForLast(s.Last)
}

func (t *PersistentTracker) statusForLast(l peerdb.LastStatus) api.KnownPeerStatus {
	switch {
	case l.Attempt == nil:
		// No connection attempted
		return api.PeerStatusIsUnknown

	case l.Success == nil:
		// No successful connection

		// Attempt was too long ago?
		if t.attemptIsTooOld(l) {
			return api.PeerStatusIsKnownBad
		}

		// Attempt was recent
		return api.PeerStatusIsUnknown

	case l.Attempt.After(*l.Success):
		// Connection attempted since the last success

		// Attempt was too long ago?
		if t.attemptIsTooOld(l) {
			return api.PeerStatusIsKnownBad
		}

		// Last success was too long ago?
		return t.statusForLastSuccess(l)

	default:
		// Last attempt was successful

		// Was it too long ago?
		return t.statusForLastSuccess(l)
	}
}

func (t *PersistentTracker) attemptIsTooOld(l peerdb.LastStatus) bool {
	return time.Since(*l.Attempt) > time.Second
}

func (t *PersistentTracker) statusForLastSuccess(l peerdb.LastStatus) api.KnownPeerStatus {
	// Last success was too long ago?
	if time.Since(*l.Success) > t.successThreshold {
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
		if t.statusForLast(s.Last) == status {
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
	if netName == "" {
		netName = t.network
	}

	var peers []peer.ID
	for _, p := range t.db.Peers() {
		s := p.Network(netName).Service(service)
		if t.statusForLast(s.Last) == status {
			peers = append(peers, p.ID)
		}
	}
	return peers
}
