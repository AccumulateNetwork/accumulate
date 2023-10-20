// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"net"
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
	t.runJob(t.scanPeers, opts.ScanFrequency, defaultScanFrequency, false)

	return t, nil
}

func (t *PersistentTracker) DB() *peerdb.DB { return t.db }

func (t *PersistentTracker) ScanPeers(duration time.Duration) {
	t.scanPeers(duration)
	t.writeDb(0)
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

	// Update the last scan time
	now := time.Now()
	defer func() { t.db.LastScan = &now }()

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

	// Wait for the scans to complete
	wg.Wait()
}

func (t *PersistentTracker) scanPeer(ctx context.Context, peer peer.AddrInfo) {
	slog.DebugCtx(ctx, "Scanning peer", "id", peer.ID)

	// TODO Check addresses

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	t.scanPeerAddresses(ctx, peer)
	t.scanPeerServices(ctx, peer)
}

func (t *PersistentTracker) scanPeerAddresses(ctx context.Context, peer peer.AddrInfo) {
	for _, addr := range peer.Addrs {
		// Ignore private IPs
		if s, err := addr.ValueForProtocol(multiaddr.P_IP4); err == nil {
			if ip := net.ParseIP(s); ip != nil && isPrivateIP(ip) {
				continue
			}
		}

		t.db.Peer(peer.ID).Address(addr).Last.DidAttempt()
		_, err := t.host.Connect(ctx, &ConnectionRequest{
			Service:  api.ServiceTypeNode.Address(),
			PeerID:   peer.ID,
			PeerAddr: addr,
		})
		if err != nil {
			slog.InfoCtx(ctx, "Unable to connect to peer", "peer", peer.ID, "error", err, "address", addr)
			continue
		}
		t.db.Peer(peer.ID).Address(addr).Last.DidSucceed()
	}
}

func (t *PersistentTracker) scanPeerServices(ctx context.Context, peer peer.AddrInfo) {
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
		slog.InfoCtx(ctx, "Unable to connect to peer", "peer", peer.ID, "error", err)
		return
	}
	t.db.Peer(peer.ID).Network(t.network).Service(creq.Service).Last.DidSucceed()

	err = s.Write(&message.NodeInfoRequest{})
	if err != nil {
		slog.InfoCtx(ctx, "Failed to request node info", "peer", peer.ID, "error", err)
		return
	}
	res, err := s.Read()
	if err != nil {
		slog.InfoCtx(ctx, "Failed to request node info", "peer", peer.ID, "error", err)
		return
	}
	var ni *message.NodeInfoResponse
	switch res := res.(type) {
	case *message.ErrorResponse:
		slog.InfoCtx(ctx, "Failed to request node info", "peer", peer.ID, "error", res.Error)
		return
	case *message.NodeInfoResponse:
		ni = res
	default:
		slog.InfoCtx(ctx, "Invalid node info response", "peer", peer.ID, "want", message.TypeNodeInfoResponse, "got", res.Type())
		return
	}

	for _, svc := range ni.Value.Services {
		slog.DebugCtx(ctx, "Attempting to conenct to service", "id", peer.ID, "service", svc)

		t.db.Peer(peer.ID).Network(t.network).Service(svc).Last.DidAttempt()
		_, err := t.host.Connect(ctx, &ConnectionRequest{Service: svc, PeerID: peer.ID})
		if err != nil {
			slog.InfoCtx(ctx, "Unable to connect to peer", "peer", peer.ID, "error", err)
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
	if service == nil {
		return // Cannot mark if there's no service
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
	if service == nil {
		// If there's no service, the status is unknown
		return api.PeerStatusIsUnknown
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
	// Expect connection attempts to succeed within 1 second
	return time.Since(*l.Attempt) > time.Second
}

func (t *PersistentTracker) statusForLastSuccess(l peerdb.LastStatus) api.KnownPeerStatus {
	// Last success was too long ago?
	if t.successIsTooOld(l) {
		return api.PeerStatusIsUnknown
	}

	// Last success was recent
	return api.PeerStatusIsKnownGood
}

func (t *PersistentTracker) successIsTooOld(l peerdb.LastStatus) bool {
	if l.Success == nil {
		return true
	}

	// If this is the first scan and it is not complete, assume the success is
	// valid
	if t.db.LastScan == nil {
		return false
	}

	// If the success is older than the last (complete) scan, it is invalid
	return l.Success.Before(*t.db.LastScan)
}

func (t *PersistentTracker) Next(addr multiaddr.Multiaddr, status api.KnownPeerStatus) (peer.ID, bool) {
	netName, _, service, _, err := api.UnpackAddress(addr)
	if err != nil {
		panic(err)
	}
	if service == nil {
		// Don't answer if the service is unspecified
		return "", false
	}

	// Get all the candidates with the given status
	var candidates []*peerdb.PeerStatus
	for _, p := range t.db.Peers.Load() {
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
	for _, p := range t.db.Peers.Load() {
		n := p.Network(netName)
		if service != nil {
			s := n.Service(service)
			if t.statusForLast(s.Last) == status {
				peers = append(peers, p.ID)
			}
			continue
		}

		for _, s := range n.Services.Load() {
			if t.statusForLast(s.Last) == status {
				peers = append(peers, p.ID)
				break
			}
		}
	}
	return peers
}

func (t *PersistentTracker) Discover(ctx context.Context, req *DiscoveryRequest) (DiscoveryResponse, error) {
	if req.Network == "" {
		req.Network = t.network
	}
	if req.Service == nil {
		return nil, errors.BadRequest.With("missing service")
	}

	ch := make(chan peer.AddrInfo)
	go func() {
		defer close(ch)

		for _, p := range t.db.Peers.Load() {
			if t.successIsTooOld(p.Network(req.Network).Service(req.Service).Last) {
				continue
			}

			info := peer.AddrInfo{ID: p.ID}
			for _, a := range p.Addresses.Load() {
				if t.successIsTooOld(a.Last) {
					info.Addrs = append(info.Addrs, a.Address)
				}
			}
			ch <- info
		}
	}()

	return DiscoveredPeers(ch), nil
}

func (t *PersistentTracker) Connect(ctx context.Context, req *ConnectionRequest) (message.Stream, error) {
	if req.Service == nil {
		return nil, errors.BadRequest.With("missing service")
	}
	if req.PeerID == "" {
		return nil, errors.BadRequest.With("missing peer")
	}

	// Try to provide a good address
	peer := t.db.Peer(req.PeerID)
	if req.PeerAddr == nil {
		for _, addr := range peer.Addresses.Load() {
			if t.statusForLast(addr.Last) == api.PeerStatusIsKnownGood {
				req.PeerAddr = addr.Address
				break
			}
		}
	}

	peer.Network(t.network).Service(req.Service).Last.DidAttempt()
	if req.PeerAddr != nil {
		peer.Address(req.PeerAddr).Last.DidAttempt()
	}
	s, err := t.host.Connect(ctx, req)
	if err != nil {
		return nil, err
	}
	peer.Network(t.network).Service(req.Service).Last.DidSucceed()
	if req.PeerAddr != nil {
		peer.Address(req.PeerAddr).Last.DidSucceed()
	}
	return s, nil
}
