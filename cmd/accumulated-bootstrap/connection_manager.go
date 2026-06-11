// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ConnectionConfig holds configuration for connection management
type ConnectionConfig struct {
	// MaxConnections is the maximum total connections allowed
	MaxConnections int

	// MaxConnectionsPerPeer limits connections from a single peer
	MaxConnectionsPerPeer int

	// HighWatermark triggers connection pruning when exceeded
	HighWatermark int

	// LowWatermark is the target after pruning
	LowWatermark int

	// GracePeriod is how long to wait before pruning new connections
	GracePeriod time.Duration

	// PruneInterval is how often to check for connection pruning
	PruneInterval time.Duration

	// ConnectionTimeout is the timeout for new connections
	ConnectionTimeout time.Duration
}

// DefaultConnectionConfig returns sensible defaults
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		MaxConnections:        500,
		MaxConnectionsPerPeer: 3,
		HighWatermark:         400,
		LowWatermark:          300,
		GracePeriod:           30 * time.Second,
		PruneInterval:         1 * time.Minute,
		ConnectionTimeout:     30 * time.Second,
	}
}

// ConnectionManager manages connections for the bootstrap server
type ConnectionManager struct {
	host       host.Host
	tracker    *PartitionTracker
	metrics    *MetricsCollector
	config     ConnectionConfig
	mu         sync.RWMutex
	connCounts map[peer.ID]int
	connTimes  map[peer.ID]time.Time
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(h host.Host, tracker *PartitionTracker, metrics *MetricsCollector, config ConnectionConfig) *ConnectionManager {
	cm := &ConnectionManager{
		host:       h,
		tracker:    tracker,
		metrics:    metrics,
		config:     config,
		connCounts: make(map[peer.ID]int),
		connTimes:  make(map[peer.ID]time.Time),
		stopCh:     make(chan struct{}),
	}

	// Subscribe to connection events
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF:    cm.onConnected,
		DisconnectedF: cm.onDisconnected,
	})

	return cm
}

// Start begins the connection management routines
func (cm *ConnectionManager) Start() {
	slog.Info("Starting connection manager",
		"max_connections", cm.config.MaxConnections,
		"high_watermark", cm.config.HighWatermark,
		"low_watermark", cm.config.LowWatermark)

	cm.wg.Add(1)
	go cm.pruneLoop()
}

// Stop stops the connection manager
func (cm *ConnectionManager) Stop() {
	close(cm.stopCh)
	cm.wg.Wait()
	slog.Info("Connection manager stopped")
}

// onConnected handles new connection events
func (cm *ConnectionManager) onConnected(n network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Track connection count per peer
	cm.connCounts[peerID]++
	if _, ok := cm.connTimes[peerID]; !ok {
		cm.connTimes[peerID] = time.Now()
	}

	// Check if peer has too many connections
	if cm.connCounts[peerID] > cm.config.MaxConnectionsPerPeer {
		slog.Debug("Peer exceeds max connections, will prune",
			"peer", peerID.String()[:12],
			"count", cm.connCounts[peerID],
			"max", cm.config.MaxConnectionsPerPeer)
	}

	// Check total connections
	totalConns := len(n.Conns())
	if totalConns > cm.config.HighWatermark {
		slog.Info("Connection high watermark exceeded, pruning",
			"total", totalConns,
			"watermark", cm.config.HighWatermark)
	}
}

// onDisconnected handles disconnection events
func (cm *ConnectionManager) onDisconnected(n network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.connCounts[peerID]--
	if cm.connCounts[peerID] <= 0 {
		delete(cm.connCounts, peerID)
		delete(cm.connTimes, peerID)
	}
}

// pruneLoop periodically checks and prunes connections
func (cm *ConnectionManager) pruneLoop() {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.config.PruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.pruneConnections()
		}
	}
}

// pruneConnections removes excess connections based on priority
func (cm *ConnectionManager) pruneConnections() {
	conns := cm.host.Network().Conns()
	totalConns := len(conns)

	// Only prune if above high watermark
	if totalConns <= cm.config.HighWatermark {
		return
	}

	slog.Info("Pruning connections",
		"current", totalConns,
		"high_watermark", cm.config.HighWatermark,
		"target", cm.config.LowWatermark)

	// Build list of connections with scores
	type connScore struct {
		conn  network.Conn
		score float64
	}

	scoredConns := make([]connScore, 0, len(conns))
	now := time.Now()

	cm.mu.RLock()
	for _, conn := range conns {
		peerID := conn.RemotePeer()
		score := cm.calculateConnectionScore(peerID, conn, now)
		scoredConns = append(scoredConns, connScore{conn: conn, score: score})
	}
	cm.mu.RUnlock()

	// Sort by score (lowest first - those will be pruned)
	sort.Slice(scoredConns, func(i, j int) bool {
		return scoredConns[i].score < scoredConns[j].score
	})

	// Prune connections until we reach low watermark
	pruned := 0
	target := totalConns - cm.config.LowWatermark

	for i := 0; i < len(scoredConns) && pruned < target; i++ {
		sc := scoredConns[i]

		// Skip connections in grace period
		if cm.isInGracePeriod(sc.conn.RemotePeer(), now) {
			continue
		}

		// Close the connection
		err := sc.conn.Close()
		if err != nil {
			slog.Debug("Failed to close connection",
				"peer", sc.conn.RemotePeer().String()[:12],
				"error", err)
		} else {
			pruned++
			slog.Debug("Pruned connection",
				"peer", sc.conn.RemotePeer().String()[:12],
				"score", sc.score)
		}
	}

	slog.Info("Connection pruning complete",
		"pruned", pruned,
		"remaining", totalConns-pruned)
}

// calculateConnectionScore calculates a priority score for a connection.
// Higher scores are kept, lower scores are pruned first. Scoring is based
// only on observable connection properties (direction and age) plus a
// penalty for peers holding excess connections — there is no per-peer
// reputation score.
func (cm *ConnectionManager) calculateConnectionScore(peerID peer.ID, conn network.Conn, now time.Time) float64 {
	score := 50.0 // Base score

	// Prefer outbound connections.
	if conn.Stat().Direction == network.DirOutbound {
		score += 10.0
	}

	// Prefer established connections.
	if connTime, ok := cm.connTimes[peerID]; ok {
		age := now.Sub(connTime)
		// Give bonus for connections older than 5 minutes
		if age > 5*time.Minute {
			// Max 20 points for connections >= 1 hour old
			ageBonus := float64(age.Minutes()) / 60.0 * 20.0
			if ageBonus > 20.0 {
				ageBonus = 20.0
			}
			score += ageBonus
		}
	}

	// Penalize peers with too many connections.
	if count, ok := cm.connCounts[peerID]; ok {
		if count > cm.config.MaxConnectionsPerPeer {
			score -= float64(count-cm.config.MaxConnectionsPerPeer) * 20.0
		}
	}

	return score
}

// isInGracePeriod checks if a peer is in the connection grace period
func (cm *ConnectionManager) isInGracePeriod(peerID peer.ID, now time.Time) bool {
	if connTime, ok := cm.connTimes[peerID]; ok {
		return now.Sub(connTime) < cm.config.GracePeriod
	}
	return false
}

// GetConnectionStats returns current connection statistics
func (cm *ConnectionManager) GetConnectionStats() map[string]interface{} {
	conns := cm.host.Network().Conns()

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Count connections by direction
	inbound := 0
	outbound := 0
	for _, conn := range conns {
		if conn.Stat().Direction == network.DirInbound {
			inbound++
		} else {
			outbound++
		}
	}

	// Count peers with multiple connections
	multiConnPeers := 0
	for _, count := range cm.connCounts {
		if count > 1 {
			multiConnPeers++
		}
	}

	return map[string]interface{}{
		"total":             len(conns),
		"inbound":           inbound,
		"outbound":          outbound,
		"unique_peers":      len(cm.connCounts),
		"multi_conn_peers":  multiConnPeers,
		"high_watermark":    cm.config.HighWatermark,
		"low_watermark":     cm.config.LowWatermark,
		"max_connections":   cm.config.MaxConnections,
		"near_limit":        len(conns) > cm.config.HighWatermark*90/100,
	}
}

