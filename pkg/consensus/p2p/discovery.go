// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

// DiscoveryConfig configures peer discovery.
type DiscoveryConfig struct {
	// BootstrapPeers is the list of bootstrap peer addresses.
	BootstrapPeers []peer.AddrInfo

	// DHTMode is the DHT mode (Server, Client, or Auto).
	// Server mode allows the node to help others discover peers.
	// Client mode only uses the DHT for discovery.
	DHTMode dht.ModeOpt

	// AdvertiseInterval is how often to re-advertise presence.
	AdvertiseInterval time.Duration

	// DiscoveryInterval is how often to search for new peers.
	DiscoveryInterval time.Duration
}

// ApplyDefaults fills in default values.
func (c *DiscoveryConfig) ApplyDefaults() {
	if c.DHTMode == 0 {
		c.DHTMode = dht.ModeAutoServer
	}
	if c.AdvertiseInterval <= 0 {
		c.AdvertiseInterval = 30 * time.Second
	}
	if c.DiscoveryInterval <= 0 {
		c.DiscoveryInterval = 10 * time.Second
	}
}

// Discovery handles peer discovery for DAG-BFT consensus.
type Discovery struct {
	host    host.Host
	dht     *dht.IpfsDHT
	routing *routing.RoutingDiscovery
	config  DiscoveryConfig

	// Rendezvous points we're advertising on
	mu          sync.RWMutex
	rendezvous  map[string]context.CancelFunc
	discovering map[string]context.CancelFunc

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDiscovery creates a new Discovery service.
func NewDiscovery(ctx context.Context, h host.Host, config DiscoveryConfig) (*Discovery, error) {
	config.ApplyDefaults()

	ctx, cancel := context.WithCancel(ctx)

	// Create DHT
	kademliaDHT, err := dht.New(ctx, h,
		dht.Mode(config.DHTMode),
		dht.ProtocolPrefix("/acc/consensus"),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create DHT: %w", err)
	}

	d := &Discovery{
		host:        h,
		dht:         kademliaDHT,
		routing:     routing.NewRoutingDiscovery(kademliaDHT),
		config:      config,
		rendezvous:  make(map[string]context.CancelFunc),
		discovering: make(map[string]context.CancelFunc),
		ctx:         ctx,
		cancel:      cancel,
	}

	return d, nil
}

// Bootstrap connects to bootstrap peers and initializes the DHT.
func (d *Discovery) Bootstrap(ctx context.Context) error {
	if len(d.config.BootstrapPeers) == 0 {
		slog.Debug("No bootstrap peers configured for discovery")
		return nil
	}

	slog.Info("Bootstrapping DHT",
		"peers", len(d.config.BootstrapPeers))

	// Connect to bootstrap peers
	var wg sync.WaitGroup
	var successCount int
	var mu sync.Mutex

	for _, pi := range d.config.BootstrapPeers {
		if pi.ID == d.host.ID() {
			continue
		}

		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()

			connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if err := d.host.Connect(connCtx, pi); err != nil {
				slog.Debug("Failed to connect to bootstrap peer",
					"peer", pi.ID.String()[:16],
					"error", err)
				return
			}

			mu.Lock()
			successCount++
			mu.Unlock()

			slog.Debug("Connected to bootstrap peer for discovery",
				"peer", pi.ID.String()[:16])
		}(pi)
	}

	wg.Wait()

	if successCount == 0 && len(d.config.BootstrapPeers) > 0 {
		return fmt.Errorf("failed to connect to any bootstrap peer")
	}

	// Bootstrap the DHT
	if err := d.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrap DHT: %w", err)
	}

	slog.Info("DHT bootstrap complete",
		"connectedPeers", successCount)

	return nil
}

// Advertise advertises this node at the given rendezvous point.
// This allows other nodes to discover this node.
func (d *Discovery) Advertise(rendezvous string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Already advertising
	if _, ok := d.rendezvous[rendezvous]; ok {
		return
	}

	advCtx, cancel := context.WithCancel(d.ctx)
	d.rendezvous[rendezvous] = cancel

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.advertiseLoop(advCtx, rendezvous)
	}()

	slog.Info("Started advertising",
		"rendezvous", rendezvous,
		"peerID", d.host.ID().String()[:16])
}

// advertiseLoop continuously advertises at a rendezvous point.
func (d *Discovery) advertiseLoop(ctx context.Context, rendezvous string) {
	ticker := time.NewTicker(d.config.AdvertiseInterval)
	defer ticker.Stop()

	// Advertise immediately
	util.Advertise(ctx, d.routing, rendezvous)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			util.Advertise(ctx, d.routing, rendezvous)
		}
	}
}

// StopAdvertise stops advertising at a rendezvous point.
func (d *Discovery) StopAdvertise(rendezvous string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if cancel, ok := d.rendezvous[rendezvous]; ok {
		cancel()
		delete(d.rendezvous, rendezvous)
	}
}

// FindPeers searches for peers at the given rendezvous point.
func (d *Discovery) FindPeers(ctx context.Context, rendezvous string, limit int) (<-chan peer.AddrInfo, error) {
	return d.routing.FindPeers(ctx, rendezvous, discovery.Limit(limit))
}

// StartDiscovery starts continuous discovery at a rendezvous point.
// Discovered peers are passed to the handler function.
func (d *Discovery) StartDiscovery(rendezvous string, handler func(peer.AddrInfo)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Already discovering
	if _, ok := d.discovering[rendezvous]; ok {
		return
	}

	discCtx, cancel := context.WithCancel(d.ctx)
	d.discovering[rendezvous] = cancel

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.discoveryLoop(discCtx, rendezvous, handler)
	}()

	slog.Info("Started peer discovery",
		"rendezvous", rendezvous)
}

// discoveryLoop continuously searches for peers.
func (d *Discovery) discoveryLoop(ctx context.Context, rendezvous string, handler func(peer.AddrInfo)) {
	ticker := time.NewTicker(d.config.DiscoveryInterval)
	defer ticker.Stop()

	// Discover immediately
	d.discoverOnce(ctx, rendezvous, handler)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.discoverOnce(ctx, rendezvous, handler)
		}
	}
}

// discoverOnce performs a single peer discovery round.
func (d *Discovery) discoverOnce(ctx context.Context, rendezvous string, handler func(peer.AddrInfo)) {
	discCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	peers, err := d.FindPeers(discCtx, rendezvous, 100)
	if err != nil {
		slog.Debug("Peer discovery failed",
			"rendezvous", rendezvous,
			"error", err)
		return
	}

	for pi := range peers {
		if pi.ID == d.host.ID() {
			continue // Skip self
		}
		handler(pi)
	}
}

// StopDiscovery stops continuous discovery at a rendezvous point.
func (d *Discovery) StopDiscovery(rendezvous string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if cancel, ok := d.discovering[rendezvous]; ok {
		cancel()
		delete(d.discovering, rendezvous)
	}
}

// DHT returns the underlying Kademlia DHT.
func (d *Discovery) DHT() *dht.IpfsDHT {
	return d.dht
}

// RoutingDiscovery returns the routing discovery instance.
func (d *Discovery) RoutingDiscovery() *routing.RoutingDiscovery {
	return d.routing
}

// Close shuts down discovery.
func (d *Discovery) Close() error {
	d.mu.Lock()
	// Cancel all advertising
	for _, cancel := range d.rendezvous {
		cancel()
	}
	// Cancel all discovery
	for _, cancel := range d.discovering {
		cancel()
	}
	d.mu.Unlock()

	d.cancel()
	d.wg.Wait()

	return d.dht.Close()
}

// PartitionRendezvous returns the rendezvous string for a partition.
// This is used to discover other validators in the same partition.
func PartitionRendezvous(partition string) string {
	return fmt.Sprintf("/acc/consensus/%s", partition)
}

// ParseBootstrapPeers parses bootstrap peer strings into AddrInfos.
// Each string should be a multiaddr with a /p2p/ component.
func ParseBootstrapPeers(addrs []string) ([]peer.AddrInfo, error) {
	peers := make([]peer.AddrInfo, 0, len(addrs))
	for _, addr := range addrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap address %q: %w", addr, err)
		}

		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			return nil, fmt.Errorf("extract peer info from %q: %w", addr, err)
		}

		peers = append(peers, *pi)
	}
	return peers, nil
}
