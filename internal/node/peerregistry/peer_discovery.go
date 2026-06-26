// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerregistry

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// DiscoveryConfig holds configuration for the active peer discovery
type DiscoveryConfig struct {
	// DiscoveryInterval is how often to run full discovery
	DiscoveryInterval time.Duration

	// ConnectTimeout is the timeout for connection attempts
	ConnectTimeout time.Duration

	// MaxConcurrentConnects limits simultaneous connection attempts
	MaxConcurrentConnects int
}

// DefaultDiscoveryConfig returns sensible defaults
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		DiscoveryInterval:     5 * time.Minute,
		ConnectTimeout:        30 * time.Second,
		MaxConcurrentConnects: 10,
	}
}

// ActiveDiscovery performs general peer discovery for the bootstrap
// server. It seeds from configured bootstrap peers and refreshes the DHT
// routing table; it does not target partitions — partition membership
// comes from peers' consensus-peer advertisements (#4043).
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
		"discovery_interval", ad.config.DiscoveryInterval)

	// Initial bootstrap connection
	ad.wg.Add(1)
	go func() {
		defer ad.wg.Done()
		ad.connectToBootstrapPeers()
	}()

	// Start periodic discovery
	ad.wg.Add(1)
	go ad.discoveryLoop()
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

		// Generate a random peer ID to search for. Non-cryptographic is fine
		// for DHT walks; suppress staticcheck SA1019 (the recommended
		// alternatives are for cryptographic use).
		randomBytes := make([]byte, 32)
		_, _ = rand.Read(randomBytes) //nolint:staticcheck

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
	} else {
		slog.Debug("Connected to discovered peer",
			"peer", peerID.String()[:12])
	}
}
