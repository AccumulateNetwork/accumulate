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

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerManager manages connections to peers for DAG-BFT consensus.
type PeerManager interface {
	// Bootstrap connects to bootstrap peers.
	Bootstrap(ctx context.Context, peers []peer.AddrInfo) error

	// Peers returns currently connected peer IDs.
	Peers() []peer.ID

	// PartitionPeers returns peers that are part of a specific partition.
	PartitionPeers(partition string) []peer.ID

	// NumPeers returns the number of connected peers.
	NumPeers() int

	// Close shuts down the peer manager.
	Close() error
}

// peerManager implements PeerManager.
type peerManager struct {
	host host.Host

	// Partition membership tracking
	mu             sync.RWMutex
	partitionPeers map[string]map[peer.ID]struct{}

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// NewPeerManager creates a new PeerManager.
func NewPeerManager(ctx context.Context, h host.Host) PeerManager {
	ctx, cancel := context.WithCancel(ctx)
	pm := &peerManager{
		host:           h,
		partitionPeers: make(map[string]map[peer.ID]struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Subscribe to connection events
	h.Network().Notify(&network.NotifyBundle{
		DisconnectedF: pm.onDisconnected,
	})

	return pm
}

// Bootstrap connects to the given bootstrap peers.
func (pm *peerManager) Bootstrap(ctx context.Context, peers []peer.AddrInfo) error {
	if len(peers) == 0 {
		slog.Debug("No bootstrap peers configured")
		return nil
	}

	slog.Info("Bootstrapping to peers", "count", len(peers))

	var wg sync.WaitGroup
	errCh := make(chan error, len(peers))

	for _, pi := range peers {
		if pi.ID == pm.host.ID() {
			continue // Skip self
		}

		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			if err := pm.connectWithRetry(ctx, pi); err != nil {
				errCh <- fmt.Errorf("connect to %s: %w", pi.ID.String()[:16], err)
			} else {
				slog.Info("Connected to bootstrap peer", "peer", pi.ID.String()[:16])
			}
		}(pi)
	}

	wg.Wait()
	close(errCh)

	// Collect errors but don't fail if at least some connections succeeded
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	connectedCount := len(peers) - len(errs)
	if connectedCount > 0 {
		slog.Info("Bootstrap complete",
			"connected", connectedCount,
			"failed", len(errs))
		return nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to connect to any bootstrap peer: %v", errs[0])
	}

	return nil
}

// connectWithRetry attempts to connect to a peer with exponential backoff.
func (pm *peerManager) connectWithRetry(ctx context.Context, pi peer.AddrInfo) error {
	const maxRetries = 3
	backoff := 100 * time.Millisecond

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := pm.host.Connect(connCtx, pi)
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err

		// Exponential backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return lastErr
}

// Peers returns all connected peer IDs.
func (pm *peerManager) Peers() []peer.ID {
	return pm.host.Network().Peers()
}

// PartitionPeers returns peers that are known to be part of a partition.
func (pm *peerManager) PartitionPeers(partition string) []peer.ID {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peerSet, ok := pm.partitionPeers[partition]
	if !ok {
		return nil
	}

	peers := make([]peer.ID, 0, len(peerSet))
	for pid := range peerSet {
		// Only include if still connected
		if pm.host.Network().Connectedness(pid) == network.Connected {
			peers = append(peers, pid)
		}
	}
	return peers
}

// RegisterPartitionPeer registers a peer as being part of a partition.
func (pm *peerManager) RegisterPartitionPeer(partition string, peerID peer.ID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.partitionPeers[partition] == nil {
		pm.partitionPeers[partition] = make(map[peer.ID]struct{})
	}
	pm.partitionPeers[partition][peerID] = struct{}{}
}

// UnregisterPartitionPeer removes a peer from a partition.
func (pm *peerManager) UnregisterPartitionPeer(partition string, peerID peer.ID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.partitionPeers[partition] != nil {
		delete(pm.partitionPeers[partition], peerID)
	}
}

// NumPeers returns the number of connected peers.
func (pm *peerManager) NumPeers() int {
	return len(pm.host.Network().Peers())
}

// Close shuts down the peer manager.
func (pm *peerManager) Close() error {
	pm.cancel()
	return nil
}

// onDisconnected is called when a peer disconnects.
func (pm *peerManager) onDisconnected(_ network.Network, _ network.Conn) {
	// Clean up is done lazily in PartitionPeers
}

// PeerInfo contains information about a connected peer.
type PeerInfo struct {
	ID        peer.ID
	Addrs     []string
	Partition string
	Connected bool
	Latency   time.Duration
}

// GetPeerInfo returns information about a specific peer.
func (pm *peerManager) GetPeerInfo(peerID peer.ID) PeerInfo {
	info := PeerInfo{
		ID:        peerID,
		Connected: pm.host.Network().Connectedness(peerID) == network.Connected,
		Latency:   pm.host.Peerstore().LatencyEWMA(peerID),
	}

	for _, addr := range pm.host.Peerstore().Addrs(peerID) {
		info.Addrs = append(info.Addrs, addr.String())
	}

	// Find partition membership
	pm.mu.RLock()
	for partition, peers := range pm.partitionPeers {
		if _, ok := peers[peerID]; ok {
			info.Partition = partition
			break
		}
	}
	pm.mu.RUnlock()

	return info
}

// GetAllPeerInfo returns information about all connected peers.
func (pm *peerManager) GetAllPeerInfo() []PeerInfo {
	peers := pm.Peers()
	infos := make([]PeerInfo, 0, len(peers))
	for _, pid := range peers {
		infos = append(infos, pm.GetPeerInfo(pid))
	}
	return infos
}
