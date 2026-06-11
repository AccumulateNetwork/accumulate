// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

// PeerPartitionInfo stores information about a tracked peer.
type PeerPartitionInfo struct {
	PeerID    peer.ID
	Addresses []multiaddr.Multiaddr
	Protocols []protocol.ID
	FirstSeen time.Time
	LastSeen  time.Time
}

// PartitionTracker tracks peers and the CometBFT endpoints they
// advertise per partition (#4043).
type PartitionTracker struct {
	host  host.Host
	mu    sync.RWMutex
	peers map[peer.ID]*PeerPartitionInfo
	// consensusByPartition holds CometBFT endpoints learned from peers'
	// consensus-peer advertisements (#4043), keyed by lowercased
	// partition ID then peer ID. This is the authoritative per-partition
	// view: a node advertises the exact per-partition CometBFT host:port,
	// which protocol-ID derivation cannot disambiguate for dual nodes.
	consensusByPartition map[string]map[peer.ID]consensuspeer.Peer
	metrics              *MetricsCollector
	notifiee             *network.NotifyBundle
	stopCh               chan struct{}
	consensusTick        *time.Ticker
	pruneTick            *time.Ticker
}

// NewPartitionTracker creates a new partition tracker.
func NewPartitionTracker(h host.Host, metrics *MetricsCollector) *PartitionTracker {
	pt := &PartitionTracker{
		host:                 h,
		peers:                make(map[peer.ID]*PeerPartitionInfo),
		consensusByPartition: make(map[string]map[peer.ID]consensuspeer.Peer),
		metrics:              metrics,
		stopCh:               make(chan struct{}),
	}

	// Track peers via libp2p connect/disconnect notifications rather than
	// guessing from protocol IDs. A connected peer is added (or refreshed);
	// a disconnected peer is forgotten promptly.
	if h != nil {
		pt.notifiee = &network.NotifyBundle{
			ConnectedF: func(_ network.Network, conn network.Conn) {
				pt.onConnected(conn.RemotePeer())
			},
			DisconnectedF: func(_ network.Network, conn network.Conn) {
				// A peer may have multiple connections; only forget it
				// once it is fully disconnected.
				pid := conn.RemotePeer()
				if pt.host.Network().Connectedness(pid) != network.Connected {
					pt.RemovePeer(pid)
				}
			},
		}
		h.Network().Notify(pt.notifiee)
	}

	// Consensus-peer advertisements change with the validator set and
	// IPs, so probe them frequently.
	pt.consensusTick = time.NewTicker(15 * time.Second)
	go pt.consensusProbeLoop()

	// Safety net: prune anything the disconnect notifier somehow missed.
	pt.pruneTick = time.NewTicker(5 * time.Minute)
	go pt.pruneLoop()

	return pt
}

// Stop stops the partition tracker.
func (pt *PartitionTracker) Stop() {
	if pt.notifiee != nil && pt.host != nil {
		pt.host.Network().StopNotify(pt.notifiee)
	}
	close(pt.stopCh)
	if pt.consensusTick != nil {
		pt.consensusTick.Stop()
	}
	if pt.pruneTick != nil {
		pt.pruneTick.Stop()
	}
}

// onConnected adds or refreshes a peer when libp2p reports a connection.
func (pt *PartitionTracker) onConnected(peerID peer.ID) {
	now := time.Now()

	pt.mu.Lock()
	defer pt.mu.Unlock()

	info, exists := pt.peers[peerID]
	if !exists {
		info = &PeerPartitionInfo{
			PeerID:    peerID,
			FirstSeen: now,
		}
		pt.peers[peerID] = info
	}
	info.Addresses = pt.host.Peerstore().Addrs(peerID)
	if protocols, err := pt.host.Peerstore().GetProtocols(peerID); err == nil {
		info.Protocols = protocols
	}
	info.LastSeen = now
}

// consensusProbeLoop periodically reads consensus-peer advertisements
// from connected peers (#4043).
func (pt *PartitionTracker) consensusProbeLoop() {
	// Probe once shortly after start so the broker converges without
	// waiting a full tick.
	pt.probeAllConsensus()
	for {
		select {
		case <-pt.stopCh:
			return
		case <-pt.consensusTick.C:
			pt.probeAllConsensus()
		}
	}
}

// pruneLoop is a slow safety net that forgets peers the disconnect
// notifier missed.
func (pt *PartitionTracker) pruneLoop() {
	for {
		select {
		case <-pt.stopCh:
			return
		case <-pt.pruneTick.C:
			pt.PruneStalePeers(15 * time.Minute)
		}
	}
}

// probeAllConsensus opens the consensus-peer advertise stream on every
// connected peer concurrently with a bounded worker pool (#4043, H1).
func (pt *PartitionTracker) probeAllConsensus() {
	peers := pt.host.Network().Peers()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, peerID := range peers {
		wg.Add(1)
		sem <- struct{}{}
		go func(pid peer.ID) {
			defer wg.Done()
			defer func() { <-sem }()
			pt.probeConsensusAdvertise(pid)
		}(peerID)
	}
	wg.Wait()

	pt.updateMetrics()
}

// probeConsensusAdvertise reads one peer's consensus-peer advertisement
// and records its CometBFT endpoint for each partition it serves. The
// CometBFT node ID is derived from the peer's libp2p key (the stream is
// libp2p-authenticated), so only the host:port is taken from the peer.
//
// Each successful probe is the authoritative full state for that peer, so
// the peer's prior consensus entries are dropped before the new ones are
// recorded (C2). The stream is size-limited and the advertised peer count
// is capped to bound resource use (C1).
func (pt *PartitionTracker) probeConsensusAdvertise(peerID peer.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	id, err := consensuspeer.NodeIDFromPeerID(peerID)
	if err != nil {
		slog.Debug("Consensus probe: cannot derive node ID", "peer", peerID, "error", err)
		return
	}

	s, err := pt.host.NewStream(ctx, peerID, consensuspeer.ProtocolID)
	if err != nil {
		slog.Debug("Consensus probe: stream failed", "peer", peerID, "error", err)
		return
	}
	defer func() { _ = s.Close() }()

	var adv consensuspeer.Advertisement
	if err := json.NewDecoder(io.LimitReader(s, 64<<10)).Decode(&adv); err != nil {
		slog.Debug("Consensus probe: decode failed", "peer", peerID, "error", err)
		return
	}
	if len(adv.Peers) > 64 {
		slog.Warn("Consensus probe: advertisement too large, ignoring",
			"peer", peerID, "count", len(adv.Peers))
		return
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Each probe is the authoritative full state for this peer: drop its
	// prior entries so stale host:port/partitions don't persist.
	pt.forgetConsensusPeer(peerID)

	for _, a := range adv.Peers {
		if a.Host == "" || a.Port == 0 || isUnroutableHost(a.Host) {
			continue
		}
		key := strings.ToLower(a.Partition)
		if pt.consensusByPartition[key] == nil {
			pt.consensusByPartition[key] = make(map[peer.ID]consensuspeer.Peer)
		}
		pt.consensusByPartition[key][peerID] = consensuspeer.Peer{ID: id, Host: a.Host, Port: a.Port}
	}
}

// RemovePeer removes a peer from the tracker.
func (pt *PartitionTracker) RemovePeer(peerID peer.ID) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	_, exists := pt.peers[peerID]
	delete(pt.peers, peerID)
	pt.forgetConsensusPeer(peerID)

	if exists {
		pt.updateMetrics()
	}
}

// forgetConsensusPeer drops a peer from every consensus partition. The
// caller must hold pt.mu.
func (pt *PartitionTracker) forgetConsensusPeer(peerID peer.ID) {
	for _, byPeer := range pt.consensusByPartition {
		delete(byPeer, peerID)
	}
}

// GetPeersByPartition returns the tracked peers that advertised a
// CometBFT endpoint for the given partition (#4043). Membership comes
// from consensusByPartition, not from any protocol-ID guess.
func (pt *PartitionTracker) GetPeersByPartition(partition string) []PeerPartitionInfo {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	byPeer, ok := pt.consensusByPartition[strings.ToLower(partition)]
	if !ok {
		return nil
	}

	result := make([]PeerPartitionInfo, 0, len(byPeer))
	for peerID := range byPeer {
		if info, exists := pt.peers[peerID]; exists {
			result = append(result, *info)
			continue
		}
		// The peer advertised this partition but isn't in the peers map
		// (e.g. transient connection state); synthesize a minimal entry.
		result = append(result, PeerPartitionInfo{
			PeerID:    peerID,
			Addresses: pt.host.Peerstore().Addrs(peerID),
		})
	}

	return result
}

// GetConsensusPeers returns the CometBFT consensus peers
// (`NodeID@host:port`) for a partition, as learned from peers'
// consensus-peer advertisements (#4043). The CometBFT node ID is derived
// from each peer's libp2p key; the host:port is what that peer
// advertised for this partition, so dual nodes resolve to the correct
// per-partition endpoint. Returns nil for an unknown partition.
func (pt *PartitionTracker) GetConsensusPeers(partition string) []consensuspeer.Peer {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	byPeer, ok := pt.consensusByPartition[strings.ToLower(partition)]
	if !ok {
		return nil
	}
	out := make([]consensuspeer.Peer, 0, len(byPeer))
	for _, p := range byPeer {
		out = append(out, p)
	}
	return out
}

// isUnroutableHost reports whether host is one a peer should not be
// dialed at — loopback or the unspecified address. DNS names and any
// other IP are treated as routable.
func isUnroutableHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false // hostname; assume routable
	}
	return ip.IsLoopback() || ip.IsUnspecified()
}

// GetPartitionStats returns statistics for all partitions, derived from
// the per-partition consensus advertisements (#4043).
func (pt *PartitionTracker) GetPartitionStats() map[string]PartitionStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	stats := make(map[string]PartitionStats)

	for partition, byPeer := range pt.consensusByPartition {
		connected := 0
		for peerID := range byPeer {
			if pt.host != nil && pt.host.Network().Connectedness(peerID) == network.Connected {
				connected++
			}
		}

		stats[partition] = PartitionStats{
			TotalPeers:     len(byPeer),
			ConnectedPeers: connected,
		}
	}

	return stats
}

// PartitionStats contains statistics for a partition.
type PartitionStats struct {
	TotalPeers     int `json:"total_peers"`
	ConnectedPeers int `json:"connected_peers"`
}

// GetAllPeers returns all tracked peers.
func (pt *PartitionTracker) GetAllPeers() []PeerPartitionInfo {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make([]PeerPartitionInfo, 0, len(pt.peers))
	for _, info := range pt.peers {
		result = append(result, *info)
	}

	return result
}

// updateMetrics updates the metrics collector with current partition
// counts derived from consensusByPartition.
func (pt *PartitionTracker) updateMetrics() {
	if pt.metrics == nil {
		return
	}

	for partition, byPeer := range pt.consensusByPartition {
		peers := make([]peer.ID, 0, len(byPeer))
		for peerID := range byPeer {
			peers = append(peers, peerID)
		}
		pt.metrics.SetPartitionPeers(partition, peers)
	}
}

// PruneStalePeers removes peers that haven't been seen recently. With the
// disconnect notifier in place this is only a safety net.
func (pt *PartitionTracker) PruneStalePeers(maxAge time.Duration) int {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0

	for peerID, info := range pt.peers {
		// Never prune a peer that is still connected — LastSeen is only
		// refreshed on connection events, so a long-lived connection has a
		// stale LastSeen but is not stale. This is purely a safety net for
		// disconnects the notifier missed.
		if pt.host != nil && pt.host.Network().Connectedness(peerID) == network.Connected {
			continue
		}
		if info.LastSeen.Before(cutoff) {
			delete(pt.peers, peerID)
			pt.forgetConsensusPeer(peerID)
			pruned++
		}
	}

	if pruned > 0 {
		slog.Info("Pruned stale peers", "count", pruned)
		pt.updateMetrics()
	}

	return pruned
}
