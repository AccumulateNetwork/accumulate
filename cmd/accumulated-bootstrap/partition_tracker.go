// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// Known partitions in the Accumulate network
const (
	PartitionDN      = "dn"
	PartitionCyclops = "cyclops"
	PartitionUnknown = "unknown"
)

// PeerPartitionInfo stores information about a peer's partition membership
type PeerPartitionInfo struct {
	PeerID     peer.ID
	Partition  string
	Addresses  []multiaddr.Multiaddr
	Protocols  []protocol.ID
	FirstSeen  time.Time
	LastSeen   time.Time
	LastProbed time.Time
	ProbeCount int
	Score      float64
}

// PartitionTracker tracks peers by their partition membership
type PartitionTracker struct {
	host      host.Host
	mu        sync.RWMutex
	peers     map[peer.ID]*PeerPartitionInfo
	byPartition map[string]map[peer.ID]struct{}
	metrics   *MetricsCollector
	stopCh    chan struct{}
	probeTick *time.Ticker
}

// NewPartitionTracker creates a new partition tracker
func NewPartitionTracker(h host.Host, metrics *MetricsCollector) *PartitionTracker {
	pt := &PartitionTracker{
		host:        h,
		peers:       make(map[peer.ID]*PeerPartitionInfo),
		byPartition: make(map[string]map[peer.ID]struct{}),
		metrics:     metrics,
		stopCh:      make(chan struct{}),
	}

	// Initialize partition maps
	pt.byPartition[PartitionDN] = make(map[peer.ID]struct{})
	pt.byPartition[PartitionCyclops] = make(map[peer.ID]struct{})
	pt.byPartition[PartitionUnknown] = make(map[peer.ID]struct{})

	// Start periodic probing
	pt.probeTick = time.NewTicker(5 * time.Minute)
	go pt.probeLoop()

	return pt
}

// Stop stops the partition tracker
func (pt *PartitionTracker) Stop() {
	close(pt.stopCh)
	if pt.probeTick != nil {
		pt.probeTick.Stop()
	}
}

// probeLoop periodically probes peers to update their partition information
func (pt *PartitionTracker) probeLoop() {
	for {
		select {
		case <-pt.stopCh:
			return
		case <-pt.probeTick.C:
			pt.probeAllPeers()
		}
	}
}

// probeAllPeers probes all connected peers to determine their partitions
func (pt *PartitionTracker) probeAllPeers() {
	peers := pt.host.Network().Peers()
	slog.Info("Probing peers for partition info", "peer_count", len(peers))

	for _, peerID := range peers {
		pt.probePeer(peerID)
	}

	// Update metrics
	pt.updateMetrics()
}

// probePeer probes a single peer to determine its partition
func (pt *PartitionTracker) probePeer(peerID peer.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get protocols supported by this peer
	protocols, err := pt.host.Peerstore().GetProtocols(peerID)
	if err != nil {
		slog.Debug("Failed to get protocols for peer", "peer", peerID, "error", err)
		return
	}

	// Determine partition from protocols
	partition := pt.detectPartition(protocols)

	// Get addresses
	addrs := pt.host.Peerstore().Addrs(peerID)

	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()

	info, exists := pt.peers[peerID]
	if !exists {
		info = &PeerPartitionInfo{
			PeerID:    peerID,
			FirstSeen: now,
			Score:     50, // Start with neutral score
		}
		pt.peers[peerID] = info
	}

	// Update peer info
	oldPartition := info.Partition
	info.Partition = partition
	info.Addresses = addrs
	info.Protocols = protocols
	info.LastSeen = now
	info.LastProbed = now
	info.ProbeCount++

	// Update partition index if changed
	if oldPartition != partition {
		if oldPartition != "" {
			delete(pt.byPartition[oldPartition], peerID)
		}
		if pt.byPartition[partition] == nil {
			pt.byPartition[partition] = make(map[peer.ID]struct{})
		}
		pt.byPartition[partition][peerID] = struct{}{}
	}

	_ = ctx // Used for future enhancements
}

// detectPartition attempts to detect a peer's partition from its protocols
func (pt *PartitionTracker) detectPartition(protocols []protocol.ID) string {
	for _, p := range protocols {
		ps := string(p)
		lower := strings.ToLower(ps)

		// Check for Accumulate service protocols
		if strings.Contains(lower, "/acc/") {
			// Look for network identifiers
			if strings.Contains(lower, "/directory/") || strings.Contains(lower, "/dn/") {
				return PartitionDN
			}
			if strings.Contains(lower, "/cyclops/") || strings.Contains(lower, "/bvn0/") {
				return PartitionCyclops
			}
		}

		// Check for known partition patterns in protocol IDs
		if strings.Contains(lower, "directory") {
			return PartitionDN
		}
		if strings.Contains(lower, "cyclops") {
			return PartitionCyclops
		}
	}

	return PartitionUnknown
}

// AddPeer adds or updates a peer in the tracker
func (pt *PartitionTracker) AddPeer(peerID peer.ID, partition string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()

	info, exists := pt.peers[peerID]
	if !exists {
		info = &PeerPartitionInfo{
			PeerID:    peerID,
			FirstSeen: now,
			Score:     50,
		}
		pt.peers[peerID] = info
	}

	oldPartition := info.Partition
	info.Partition = partition
	info.LastSeen = now

	// Update partition index if changed
	if oldPartition != partition {
		if oldPartition != "" {
			delete(pt.byPartition[oldPartition], peerID)
		}
		if pt.byPartition[partition] == nil {
			pt.byPartition[partition] = make(map[peer.ID]struct{})
		}
		pt.byPartition[partition][peerID] = struct{}{}
	}

	pt.updateMetrics()
}

// RemovePeer removes a peer from the tracker
func (pt *PartitionTracker) RemovePeer(peerID peer.ID) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	info, exists := pt.peers[peerID]
	if !exists {
		return
	}

	if info.Partition != "" {
		delete(pt.byPartition[info.Partition], peerID)
	}
	delete(pt.peers, peerID)

	pt.updateMetrics()
}

// GetPeersByPartition returns all peers for a given partition
func (pt *PartitionTracker) GetPeersByPartition(partition string) []PeerPartitionInfo {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	partition = strings.ToLower(partition)
	peerIDs, ok := pt.byPartition[partition]
	if !ok {
		return nil
	}

	result := make([]PeerPartitionInfo, 0, len(peerIDs))
	for peerID := range peerIDs {
		if info, exists := pt.peers[peerID]; exists {
			result = append(result, *info)
		}
	}

	return result
}

// GetPartitionStats returns statistics for all partitions
func (pt *PartitionTracker) GetPartitionStats() map[string]PartitionStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	stats := make(map[string]PartitionStats)

	for partition, peerIDs := range pt.byPartition {
		var totalScore float64
		var connected int

		for peerID := range peerIDs {
			info := pt.peers[peerID]
			if info != nil {
				totalScore += info.Score

				// Check if currently connected
				if pt.host.Network().Connectedness(peerID) == 1 { // Connected
					connected++
				}
			}
		}

		avgScore := 0.0
		if len(peerIDs) > 0 {
			avgScore = totalScore / float64(len(peerIDs))
		}

		stats[partition] = PartitionStats{
			TotalPeers:     len(peerIDs),
			ConnectedPeers: connected,
			AverageScore:   avgScore,
		}
	}

	return stats
}

// PartitionStats contains statistics for a partition
type PartitionStats struct {
	TotalPeers     int     `json:"total_peers"`
	ConnectedPeers int     `json:"connected_peers"`
	AverageScore   float64 `json:"average_score"`
}

// GetAllPeers returns all tracked peers
func (pt *PartitionTracker) GetAllPeers() []PeerPartitionInfo {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make([]PeerPartitionInfo, 0, len(pt.peers))
	for _, info := range pt.peers {
		result = append(result, *info)
	}

	return result
}

// UpdateScore updates a peer's score
func (pt *PartitionTracker) UpdateScore(peerID peer.ID, delta float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	info, exists := pt.peers[peerID]
	if !exists {
		return
	}

	// Update score with bounds
	info.Score += delta
	if info.Score < 0 {
		info.Score = 0
	}
	if info.Score > 100 {
		info.Score = 100
	}
}

// RecordSuccess records a successful interaction with a peer
func (pt *PartitionTracker) RecordSuccess(peerID peer.ID) {
	pt.UpdateScore(peerID, 1)
}

// RecordFailure records a failed interaction with a peer
func (pt *PartitionTracker) RecordFailure(peerID peer.ID) {
	pt.UpdateScore(peerID, -2)
}

// updateMetrics updates the metrics collector with current partition counts
func (pt *PartitionTracker) updateMetrics() {
	if pt.metrics == nil {
		return
	}

	for partition, peerIDs := range pt.byPartition {
		peers := make([]peer.ID, 0, len(peerIDs))
		for peerID := range peerIDs {
			peers = append(peers, peerID)
		}
		pt.metrics.SetPartitionPeers(partition, peers)
	}
}

// PruneStalePeers removes peers that haven't been seen recently
func (pt *PartitionTracker) PruneStalePeers(maxAge time.Duration) int {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0

	for peerID, info := range pt.peers {
		if info.LastSeen.Before(cutoff) {
			if info.Partition != "" {
				delete(pt.byPartition[info.Partition], peerID)
			}
			delete(pt.peers, peerID)
			pruned++
		}
	}

	if pruned > 0 {
		slog.Info("Pruned stale peers", "count", pruned)
		pt.updateMetrics()
	}

	return pruned
}
