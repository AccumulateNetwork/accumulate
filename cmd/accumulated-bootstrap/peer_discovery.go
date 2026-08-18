// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	cryptorand "crypto/rand"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// DiscoveryConfig holds configuration for the active peer discovery
type DiscoveryConfig struct {
	// DiscoveryInterval is how often to run full discovery
	DiscoveryInterval time.Duration

	// ProbeInterval is how often to probe known peers
	ProbeInterval time.Duration

	// ConnectTimeout is the timeout for connection attempts
	ConnectTimeout time.Duration

	// MaxConcurrentConnects limits simultaneous connection attempts
	MaxConcurrentConnects int

	// MinPeersPerPartition is the target minimum peers per partition
	MinPeersPerPartition int

	// MaxPeersToProbe limits how many peers to probe per cycle
	MaxPeersToProbe int
}

// DefaultDiscoveryConfig returns sensible defaults
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		DiscoveryInterval:     5 * time.Minute,
		ProbeInterval:         2 * time.Minute,
		ConnectTimeout:        30 * time.Second,
		MaxConcurrentConnects: 10,
		MinPeersPerPartition:  5,
		MaxPeersToProbe:       50,
	}
}

// ActiveDiscovery performs active peer discovery for the bootstrap server
type ActiveDiscovery struct {
	host    host.Host
	dht     *dht.IpfsDHT
	tracker *PartitionTracker
	metrics *MetricsCollector
	config  DiscoveryConfig
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// Known bootstrap peers to seed discovery
	bootstrapPeers []peer.AddrInfo
}

// NewActiveDiscovery creates a new active discovery service
func NewActiveDiscovery(h host.Host, d *dht.IpfsDHT, tracker *PartitionTracker, metrics *MetricsCollector, config DiscoveryConfig) *ActiveDiscovery {
	return &ActiveDiscovery{
		host:    h,
		dht:     d,
		tracker: tracker,
		metrics: metrics,
		config:  config,
		stopCh:  make(chan struct{}),
	}
}

// SetBootstrapPeers sets the initial bootstrap peers to connect to
func (ad *ActiveDiscovery) SetBootstrapPeers(peers []peer.AddrInfo) {
	ad.bootstrapPeers = peers
}

// Start begins the active discovery process
func (ad *ActiveDiscovery) Start() {
	slog.Info("Starting active peer discovery",
		"discovery_interval", ad.config.DiscoveryInterval,
		"probe_interval", ad.config.ProbeInterval)

	// Initial bootstrap connection
	ad.wg.Add(1)
	go func() {
		defer ad.wg.Done()
		ad.connectToBootstrapPeers()
	}()

	// Start periodic discovery
	ad.wg.Add(1)
	go ad.discoveryLoop()

	// Start periodic probing
	ad.wg.Add(1)
	go ad.probeLoop()
}

// Stop stops the active discovery process
func (ad *ActiveDiscovery) Stop() {
	close(ad.stopCh)
	ad.wg.Wait()
	slog.Info("Active peer discovery stopped")
}

// connectToBootstrapPeers connects to known bootstrap peers
func (ad *ActiveDiscovery) connectToBootstrapPeers() {
	if len(ad.bootstrapPeers) == 0 {
		slog.Debug("No bootstrap peers configured")
		return
	}

	slog.Info("Connecting to bootstrap peers", "count", len(ad.bootstrapPeers))

	for _, peerInfo := range ad.bootstrapPeers {
		select {
		case <-ad.stopCh:
			return
		default:
		}

		if peerInfo.ID == ad.host.ID() {
			continue // Skip self
		}

		ctx, cancel := context.WithTimeout(context.Background(), ad.config.ConnectTimeout)
		err := ad.host.Connect(ctx, peerInfo)
		cancel()

		if err != nil {
			slog.Debug("Failed to connect to bootstrap peer",
				"peer", peerInfo.ID.String()[:12],
				"error", err)
		} else {
			slog.Info("Connected to bootstrap peer",
				"peer", peerInfo.ID.String()[:12],
				"addrs", len(peerInfo.Addrs))
		}
	}
}

// discoveryLoop runs periodic DHT discovery
func (ad *ActiveDiscovery) discoveryLoop() {
	defer ad.wg.Done()

	// Run initial discovery after short delay
	select {
	case <-ad.stopCh:
		return
	case <-time.After(30 * time.Second):
	}

	ad.runDiscovery()

	ticker := time.NewTicker(ad.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ad.stopCh:
			return
		case <-ticker.C:
			ad.runDiscovery()
		}
	}
}

// runDiscovery performs a full DHT discovery cycle
func (ad *ActiveDiscovery) runDiscovery() {
	start := time.Now()
	slog.Info("Starting DHT discovery cycle")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Refresh the DHT routing table
	if ad.dht != nil {
		select {
		case <-ad.dht.RefreshRoutingTable():
			slog.Debug("DHT routing table refreshed")
		case <-ctx.Done():
			slog.Warn("DHT refresh timed out")
		case <-ad.stopCh:
			return
		}
	}

	// Find peers through DHT random walks
	peersFound := ad.randomWalkDiscovery(ctx)

	// Try to connect to peers in partitions that need more peers
	ad.ensurePartitionCoverage(ctx)

	duration := time.Since(start)
	slog.Info("DHT discovery cycle complete",
		"duration", duration,
		"peers_found", peersFound,
		"connected", len(ad.host.Network().Peers()))

	if ad.metrics != nil {
		ad.metrics.RecordDiscovery("dht_walk", duration, peersFound, "all")
	}
}

// randomWalkDiscovery performs random DHT walks to discover peers
func (ad *ActiveDiscovery) randomWalkDiscovery(ctx context.Context) int {
	if ad.dht == nil {
		return 0
	}

	peersFound := 0

	// Generate random peer IDs to search for (DHT random walk)
	for i := 0; i < 5; i++ {
		select {
		case <-ctx.Done():
			return peersFound
		case <-ad.stopCh:
			return peersFound
		default:
		}

		// Generate a random peer ID to search for
		randomBytes := make([]byte, 32)
		if _, err := cryptorand.Read(randomBytes); err != nil {
			slog.Error("random read failed", "error", err)
			continue
		}

		// FindPeersConnectedToPeer or GetClosestPeers does a DHT walk
		closestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		closestPeers, err := ad.dht.GetClosestPeers(closestCtx, string(randomBytes))
		cancel()

		if err != nil {
			slog.Debug("DHT random walk failed", "error", err)
			continue
		}

		for _, foundPeer := range closestPeers {
			if foundPeer != ad.host.ID() {
				peersFound++
				ad.maybeConnect(ctx, foundPeer)
			}
		}
	}

	return peersFound
}

// maybeConnect attempts to connect to a peer if not already connected
func (ad *ActiveDiscovery) maybeConnect(ctx context.Context, peerID peer.ID) {
	// Skip if already connected
	if ad.host.Network().Connectedness(peerID) == network.Connected {
		return
	}

	// Get addresses from peerstore
	addrs := ad.host.Peerstore().Addrs(peerID)
	if len(addrs) == 0 {
		return
	}

	peerInfo := peer.AddrInfo{
		ID:    peerID,
		Addrs: addrs,
	}

	connectCtx, cancel := context.WithTimeout(ctx, ad.config.ConnectTimeout)
	err := ad.host.Connect(connectCtx, peerInfo)
	cancel()

	if err != nil {
		slog.Debug("Failed to connect to discovered peer",
			"peer", peerID.String()[:12],
			"error", err)
		if ad.tracker != nil {
			ad.tracker.RecordFailure(peerID)
		}
	} else {
		slog.Debug("Connected to discovered peer",
			"peer", peerID.String()[:12])
		if ad.tracker != nil {
			ad.tracker.RecordSuccess(peerID)
		}
	}
}

// ensurePartitionCoverage tries to maintain minimum peers per partition
func (ad *ActiveDiscovery) ensurePartitionCoverage(ctx context.Context) {
	if ad.tracker == nil {
		return
	}

	stats := ad.tracker.GetPartitionStats()

	for partition, stat := range stats {
		if partition == PartitionUnknown {
			continue
		}

		if stat.ConnectedPeers < ad.config.MinPeersPerPartition {
			slog.Info("Partition needs more peers",
				"partition", partition,
				"connected", stat.ConnectedPeers,
				"target", ad.config.MinPeersPerPartition)

			// Try to connect to known peers in this partition
			peers := ad.tracker.GetPeersByPartition(partition)
			for _, peerInfo := range peers {
				select {
				case <-ctx.Done():
					return
				case <-ad.stopCh:
					return
				default:
				}

				if ad.host.Network().Connectedness(peerInfo.PeerID) != network.Connected {
					ad.maybeConnect(ctx, peerInfo.PeerID)
				}
			}
		}
	}
}

// probeLoop periodically probes connected peers
func (ad *ActiveDiscovery) probeLoop() {
	defer ad.wg.Done()

	ticker := time.NewTicker(ad.config.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ad.stopCh:
			return
		case <-ticker.C:
			ad.probeConnectedPeers()
		}
	}
}

// probeConnectedPeers probes connected peers to update their status
func (ad *ActiveDiscovery) probeConnectedPeers() {
	peers := ad.host.Network().Peers()
	if len(peers) == 0 {
		return
	}

	// Limit how many peers we probe
	toProbe := peers
	if len(toProbe) > ad.config.MaxPeersToProbe {
		// Shuffle and take first N
		rand.Shuffle(len(toProbe), func(i, j int) {
			toProbe[i], toProbe[j] = toProbe[j], toProbe[i]
		})
		toProbe = toProbe[:ad.config.MaxPeersToProbe]
	}

	slog.Debug("Probing connected peers", "count", len(toProbe))

	var wg sync.WaitGroup
	sem := make(chan struct{}, ad.config.MaxConcurrentConnects)

	for _, peerID := range toProbe {
		select {
		case <-ad.stopCh:
			break
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(pid peer.ID) {
			defer wg.Done()
			defer func() { <-sem }()

			ad.probePeer(pid)
		}(peerID)
	}

	wg.Wait()
}

// probePeer probes a single peer to update its information
func (ad *ActiveDiscovery) probePeer(peerID peer.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get protocols supported by this peer
	protocols, err := ad.host.Peerstore().GetProtocols(peerID)
	if err != nil {
		slog.Debug("Failed to get protocols", "peer", peerID.String()[:12], "error", err)
		return
	}

	// Determine partition from protocols
	partition := detectPartitionFromProtocols(protocols)

	// Update tracker
	if ad.tracker != nil {
		ad.tracker.AddPeer(peerID, partition)
		ad.tracker.RecordSuccess(peerID)
	}

	// Also learn about peers this peer knows about
	if ad.dht != nil {
		ad.learnFromPeer(ctx, peerID)
	}
}

// learnFromPeer queries a peer for other peers it knows
func (ad *ActiveDiscovery) learnFromPeer(ctx context.Context, peerID peer.ID) {
	if ad.dht == nil {
		return
	}

	// Get the peer's closest peers (peers it knows about)
	closestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Ask DHT for peers near this peer's ID
	closestPeers, err := ad.dht.GetClosestPeers(closestCtx, string(peerID))
	if err != nil {
		return
	}

	newPeers := 0
	for _, foundPeerID := range closestPeers {
		if foundPeerID != ad.host.ID() && foundPeerID != peerID {
			// Store in peerstore if we don't know this peer
			if len(ad.host.Peerstore().Addrs(foundPeerID)) == 0 {
				newPeers++
			}
		}
	}

	if newPeers > 0 {
		slog.Debug("Learned new peers from peer",
			"source", peerID.String()[:12],
			"new_peers", newPeers)
	}
}

// detectPartitionFromProtocols determines partition from protocol list
func detectPartitionFromProtocols(protocols []protocol.ID) string {
	for _, p := range protocols {
		ps := string(p)
		// Check for Accumulate service protocols
		if contains(ps, "/acc/") {
			if contains(ps, "/directory/") || contains(ps, "/dn/") {
				return PartitionDN
			}
			if contains(ps, "/cyclops/") || contains(ps, "/bvn0/") {
				return PartitionCyclops
			}
		}

		// Check for known partition patterns
		if contains(ps, "directory") {
			return PartitionDN
		}
		if contains(ps, "cyclops") {
			return PartitionCyclops
		}
	}

	return PartitionUnknown
}

// contains is a simple case-insensitive substring check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if matchLower(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func matchLower(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// GetDiscoveredPeers returns all discovered peers with their addresses
func (ad *ActiveDiscovery) GetDiscoveredPeers() []peer.AddrInfo {
	peers := ad.host.Peerstore().Peers()
	result := make([]peer.AddrInfo, 0, len(peers))

	for _, p := range peers {
		if p == ad.host.ID() {
			continue
		}

		addrs := ad.host.Peerstore().Addrs(p)
		if len(addrs) > 0 {
			result = append(result, peer.AddrInfo{
				ID:    p,
				Addrs: addrs,
			})
		}
	}

	return result
}

// GetPeerAddresses returns addresses for a specific peer
func (ad *ActiveDiscovery) GetPeerAddresses(peerID peer.ID) []multiaddr.Multiaddr {
	return ad.host.Peerstore().Addrs(peerID)
}

// AddPeerAddresses adds addresses for a peer to the peerstore
func (ad *ActiveDiscovery) AddPeerAddresses(peerID peer.ID, addrs []multiaddr.Multiaddr) {
	ad.host.Peerstore().AddAddrs(peerID, addrs, peerstore.PermanentAddrTTL)
}
