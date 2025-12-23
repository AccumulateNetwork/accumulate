// Copyright 2025 The Accumulate Authors
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

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
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
	statusProvider StatusProvider
	peerDialer     PeerDialer
	persistentPeers string

	// Configuration
	MaxBlockTimeDrift   time.Duration
	ReconnectThreshold  int // consecutive stale checks before reconnecting
	WarnThreshold       int // consecutive stale checks before warning

	// State
	staleCount int
	lastHeight int64
}

// NewSyncMonitor creates a new sync monitor
func NewSyncMonitor(status StatusProvider, dialer PeerDialer, persistentPeers string) *SyncMonitor {
	return &SyncMonitor{
		statusProvider:     status,
		peerDialer:        dialer,
		persistentPeers:   persistentPeers,
		MaxBlockTimeDrift: 10 * time.Second,
		ReconnectThreshold: 30,
		WarnThreshold:      10,
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

	// Check if we're making progress
	makingProgress := currentHeight > m.lastHeight
	m.lastHeight = currentHeight

	if makingProgress {
		slog.Debug("Node reports caught up but block time is stale (making progress)",
			"block_time", st.LatestBlockTime,
			"block_age", blockAge,
			"height", currentHeight,
			"stale_count", m.staleCount)
		m.staleCount = 0
		return CheckResultStaleProgress, nil
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
}

// daemonStatusProvider adapts the CometBFT local client to StatusProvider
type daemonStatusProvider struct {
	client interface {
		Status(ctx context.Context) (*coretypes.ResultStatus, error)
	}
}

func (p *daemonStatusProvider) Status(ctx context.Context) (*SyncStatus, error) {
	st, err := p.client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &SyncStatus{
		CatchingUp:        st.SyncInfo.CatchingUp,
		LatestBlockHeight: st.SyncInfo.LatestBlockHeight,
		LatestBlockTime:   st.SyncInfo.LatestBlockTime,
	}, nil
}

// daemonPeerDialer adapts the CometBFT switch to PeerDialer
type daemonPeerDialer struct {
	sw interface {
		DialPeersAsync(peers []string) error
	}
}

func (d *daemonPeerDialer) DialPeersAsync(peers []string) error {
	return d.sw.DialPeersAsync(peers)
}
