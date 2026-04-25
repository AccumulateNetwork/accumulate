// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// SyncStatus represents the current sync state returned by CometBFT
type SyncStatus struct {
	CatchingUp        bool
	LatestBlockHeight int64
	LatestBlockTime   time.Time
}

// StatusProvider is an interface for getting node sync status
type StatusProvider interface {
	Status(ctx context.Context) (*SyncStatus, error)
}

// PeerDialer is an interface for reconnecting to peers
type PeerDialer interface {
	DialPeersAsync(peers []string) error
}

// SyncMonitor monitors the node's sync state and attempts recovery when stuck
type SyncMonitor struct {
	statusProvider  StatusProvider
	peerDialer      PeerDialer
	persistentPeers string

	// Configuration
	MaxBlockTimeDrift  time.Duration
	ReconnectThreshold int // consecutive stale checks before reconnecting
	WarnThreshold      int // consecutive stale checks before warning
	MinFastSyncRate    int // minimum blocks/sec to count as "fast sync" vs "following"

	// State
	staleCount    int
	lastHeight    int64
	lastCheckTime time.Time
}

// NewSyncMonitor creates a new sync monitor
func NewSyncMonitor(status StatusProvider, dialer PeerDialer, persistentPeers string) *SyncMonitor {
	return &SyncMonitor{
		statusProvider:     status,
		peerDialer:         dialer,
		persistentPeers:    persistentPeers,
		MaxBlockTimeDrift:  10 * time.Second,
		ReconnectThreshold: 30,
		WarnThreshold:      10,
		MinFastSyncRate:    5, // blocks/sec; production rate is ~1 bl/sec
	}
}

// CheckResult represents the result of a sync check
type CheckResult int

const (
	CheckResultCatchingUp     CheckResult = iota // Node is actively syncing
	CheckResultSynced                            // Node is fully synced
	CheckResultStaleProgress                     // Stale but making progress
	CheckResultStaleWarning                      // Stale and stuck, warning level
	CheckResultStaleReconnect                    // Stale and stuck, attempting reconnect
)

// Check performs a single sync check and returns the result
// This is the core logic extracted for testing
func (m *SyncMonitor) Check(ctx context.Context) (CheckResult, error) {
	st, err := m.statusProvider.Status(ctx)
	if err != nil {
		return CheckResultCatchingUp, err
	}

	// If CometBFT says we're catching up, trust it
	if st.CatchingUp {
		m.staleCount = 0
		return CheckResultCatchingUp, nil
	}

	blockAge := time.Since(st.LatestBlockTime)
	currentHeight := st.LatestBlockHeight

	// Check if block time is fresh
	if blockAge <= m.MaxBlockTimeDrift {
		m.staleCount = 0
		return CheckResultSynced, nil
	}

	// Block time is stale
	m.staleCount++

	// Check if we're making progress and at what rate
	heightDelta := currentHeight - m.lastHeight
	now := time.Now()
	timeDelta := now.Sub(m.lastCheckTime)

	// Calculate progress rate (blocks per second)
	var progressRate float64
	if timeDelta > 0 && m.lastHeight > 0 {
		progressRate = float64(heightDelta) / timeDelta.Seconds()
	}

	// Update tracking state
	m.lastHeight = currentHeight
	m.lastCheckTime = now

	// Only consider it "real progress" if we're syncing faster than production rate
	// Production rate is ~1 bl/sec, fast sync should be much faster
	fastSyncing := progressRate >= float64(m.MinFastSyncRate)

	if heightDelta > 0 && fastSyncing {
		slog.Debug("Node reports caught up but block time is stale (fast syncing)",
			"block_time", st.LatestBlockTime,
			"block_age", blockAge,
			"height", currentHeight,
			"progress_rate", progressRate,
			"stale_count", m.staleCount)
		m.staleCount = 0
		return CheckResultStaleProgress, nil
	}

	// Making slow progress (just following) or no progress - don't reset stale count
	if heightDelta > 0 {
		slog.Debug("Node reports caught up but block time is stale (slow progress, not fast syncing)",
			"block_time", st.LatestBlockTime,
			"block_age", blockAge,
			"height", currentHeight,
			"progress_rate", progressRate,
			"min_fast_sync_rate", m.MinFastSyncRate,
			"stale_count", m.staleCount)
	}

	// Not making progress - problematic case
	if m.staleCount >= m.ReconnectThreshold {
		slog.Error("Node stuck: not syncing despite stale blocks, attempting peer reconnection",
			"block_time", st.LatestBlockTime,
			"block_age", blockAge,
			"height", currentHeight,
			"stale_seconds", m.staleCount)

		m.reconnectPeers()
		m.staleCount = 0
		return CheckResultStaleReconnect, nil
	}

	if m.staleCount >= m.WarnThreshold {
		slog.Warn("Node reports caught up but block time is stale and not progressing",
			"block_time", st.LatestBlockTime,
			"block_age", blockAge,
			"height", currentHeight,
			"stale_seconds", m.staleCount,
			"reconnect_in", m.ReconnectThreshold-m.staleCount)
		return CheckResultStaleWarning, nil
	}

	slog.Debug("Node reports caught up but block time is stale",
		"block_time", st.LatestBlockTime,
		"block_age", blockAge,
		"height", currentHeight,
		"stale_count", m.staleCount)
	return CheckResultStaleWarning, nil
}

func (m *SyncMonitor) reconnectPeers() {
	if m.persistentPeers == "" || m.peerDialer == nil {
		return
	}

	peerList := strings.Split(m.persistentPeers, ",")
	for i := range peerList {
		peerList[i] = strings.TrimSpace(peerList[i])
	}

	if err := m.peerDialer.DialPeersAsync(peerList); err != nil {
		slog.Error("Failed to dial persistent peers", "error", err)
	} else {
		slog.Info("Initiated reconnection to persistent peers", "count", len(peerList))
	}
}

// StaleCount returns the current stale count for testing
func (m *SyncMonitor) StaleCount() int {
	return m.staleCount
}

// Reset resets the monitor state
func (m *SyncMonitor) Reset() {
	m.staleCount = 0
	m.lastHeight = 0
	m.lastCheckTime = time.Time{}
}

