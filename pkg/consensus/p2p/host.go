// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package p2p provides P2P networking for DAG-BFT consensus using libp2p.
package p2p

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
)

// Default connection limits for the consensus network.
const (
	DefaultLowWatermark  = 100
	DefaultHighWatermark = 400
	DefaultGracePeriod   = time.Minute
)

// HostConfig configures a libp2p host for DAG-BFT consensus.
type HostConfig struct {
	// ListenAddrs is a list of multiaddrs to listen on.
	// Example: /ip4/0.0.0.0/tcp/9000
	ListenAddrs []multiaddr.Multiaddr

	// ExternalAddr is the external address to advertise.
	// If set, it replaces non-loopback addresses.
	ExternalAddr multiaddr.Multiaddr

	// PrivateKey is the ed25519 private key for the node identity.
	// If nil, a new key is generated.
	PrivateKey ed25519.PrivateKey

	// LowWatermark is the minimum number of connections to maintain.
	// Below this, the connection manager will not prune.
	LowWatermark int

	// HighWatermark is the maximum number of connections.
	// Above this, connections are pruned.
	HighWatermark int

	// GracePeriod is the minimum time a connection must be open before pruning.
	GracePeriod time.Duration
}

// ApplyDefaults fills in default values for unset fields.
func (c *HostConfig) ApplyDefaults() {
	if c.LowWatermark <= 0 {
		c.LowWatermark = DefaultLowWatermark
	}
	if c.HighWatermark <= 0 {
		c.HighWatermark = DefaultHighWatermark
	}
	if c.GracePeriod <= 0 {
		c.GracePeriod = DefaultGracePeriod
	}
}

// Host wraps a libp2p host with DAG-BFT specific functionality.
type Host struct {
	host   host.Host
	pubsub *pubsub.PubSub
	config HostConfig

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	closed bool
}

// NewHost creates a new libp2p host configured for DAG-BFT consensus.
func NewHost(ctx context.Context, config HostConfig) (*Host, error) {
	config.ApplyDefaults()

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.EnableNATService(),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
	}

	// Set listen addresses
	if len(config.ListenAddrs) > 0 {
		opts = append(opts, libp2p.ListenAddrs(config.ListenAddrs...))
	}

	// Set identity key
	if config.PrivateKey != nil {
		key, _, err := crypto.KeyPairFromStdKey(&config.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("convert private key: %w", err)
		}
		opts = append(opts, libp2p.Identity(key))
	}

	// Set connection manager for resource limits
	cm, err := connmgr.NewConnManager(
		config.LowWatermark,
		config.HighWatermark,
		connmgr.WithGracePeriod(config.GracePeriod),
	)
	if err != nil {
		return nil, fmt.Errorf("create connection manager: %w", err)
	}
	opts = append(opts, libp2p.ConnectionManager(cm))

	// Set address factory for external address
	if config.ExternalAddr != nil {
		opts = append(opts, libp2p.AddrsFactory(func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return replaceExternalAddrs(addrs, config.ExternalAddr)
		}))
	}

	// Create the libp2p host
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	// Create GossipSub pubsub
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}

	hostCtx, cancel := context.WithCancel(ctx)
	return &Host{
		host:   h,
		pubsub: ps,
		config: config,
		ctx:    hostCtx,
		cancel: cancel,
	}, nil
}

// replaceExternalAddrs replaces non-loopback IPs with the external address.
func replaceExternalAddrs(addrs []multiaddr.Multiaddr, external multiaddr.Multiaddr) []multiaddr.Multiaddr {
	result := make([]multiaddr.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		first, rest := multiaddr.SplitFirst(addr)
		if first == nil {
			result = append(result, addr)
			continue
		}

		switch first.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			ip := net.ParseIP(first.Value())
			if ip != nil && !ip.IsLoopback() {
				result = append(result, external.Encapsulate(rest))
				continue
			}
		}
		result = append(result, addr)
	}
	return result
}

// Host returns the underlying libp2p host.
func (h *Host) Host() host.Host {
	return h.host
}

// PubSub returns the GossipSub instance.
func (h *Host) PubSub() *pubsub.PubSub {
	return h.pubsub
}

// ID returns the peer ID of this host.
func (h *Host) ID() peer.ID {
	return h.host.ID()
}

// Addrs returns the listen addresses of this host.
func (h *Host) Addrs() []multiaddr.Multiaddr {
	return h.host.Addrs()
}

// AddrInfo returns the peer address info for this host.
func (h *Host) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{
		ID:    h.host.ID(),
		Addrs: h.host.Addrs(),
	}
}

// Connect connects to a peer at the given address.
func (h *Host) Connect(ctx context.Context, pi peer.AddrInfo) error {
	return h.host.Connect(ctx, pi)
}

// Peers returns the currently connected peers.
func (h *Host) Peers() []peer.ID {
	return h.host.Network().Peers()
}

// NumPeers returns the number of connected peers.
func (h *Host) NumPeers() int {
	return len(h.host.Network().Peers())
}

// Close shuts down the host and releases resources.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.cancel()
	h.mu.Unlock()

	slog.Info("Closing P2P host",
		"peerID", h.host.ID().String()[:16],
		"connectedPeers", len(h.host.Network().Peers()))

	return h.host.Close()
}

// ParseMultiaddr parses a multiaddr string.
func ParseMultiaddr(s string) (multiaddr.Multiaddr, error) {
	return multiaddr.NewMultiaddr(s)
}

// ParseMultiaddrs parses multiple multiaddr strings.
func ParseMultiaddrs(strs []string) ([]multiaddr.Multiaddr, error) {
	addrs := make([]multiaddr.Multiaddr, 0, len(strs))
	for _, s := range strs {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid multiaddr %q: %w", s, err)
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// AddrInfoFromString parses a multiaddr string into a peer.AddrInfo.
// The multiaddr must include a /p2p/ component with the peer ID.
func AddrInfoFromString(s string) (peer.AddrInfo, error) {
	addr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		return peer.AddrInfo{}, fmt.Errorf("invalid multiaddr: %w", err)
	}
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return peer.AddrInfo{}, fmt.Errorf("extract peer info: %w", err)
	}
	return *info, nil
}

// AddrInfosFromStrings parses multiple multiaddr strings into peer.AddrInfos.
func AddrInfosFromStrings(strs []string) ([]peer.AddrInfo, error) {
	infos := make([]peer.AddrInfo, 0, len(strs))
	for _, s := range strs {
		info, err := AddrInfoFromString(s)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}
