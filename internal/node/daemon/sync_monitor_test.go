// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStatusProvider implements StatusProvider for testing
type mockStatusProvider struct {
	status *SyncStatus
	err    error
}

func (m *mockStatusProvider) Status(ctx context.Context) (*SyncStatus, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.status, nil
}

// mockPeerDialer implements PeerDialer for testing
type mockPeerDialer struct {
	dialCalls [][]string
	err       error
}

func (m *mockPeerDialer) DialPeersAsync(peers []string) error {
	m.dialCalls = append(m.dialCalls, peers)
	return m.err
}

func TestSyncMonitor_CatchingUp(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        true,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Hour), // stale, but catching up
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	result, err := m.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CheckResultCatchingUp, result)
	assert.Equal(t, 0, m.StaleCount())
	assert.Empty(t, dialer.dialCalls, "should not dial when catching up")
}

func TestSyncMonitor_FullySynced(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-5 * time.Second), // fresh
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	result, err := m.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CheckResultSynced, result)
	assert.Equal(t, 0, m.StaleCount())
}

func TestSyncMonitor_StaleButMakingFastProgress(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	// Prime the monitor with initial state
	m.lastHeight = 90
	m.lastCheckTime = time.Now().Add(-1 * time.Second) // 1 second ago
	m.staleCount = 5                                   // already stale

	// Check with height 100 - that's 10 blocks in 1 second = 10 bl/sec (fast sync)
	result, err := m.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CheckResultStaleProgress, result)
	assert.Equal(t, 0, m.StaleCount(), "should reset stale count when fast syncing")
}

func TestSyncMonitor_StaleButMakingSlowProgress(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")
	m.MinFastSyncRate = 5 // 5 blocks/sec minimum for "fast sync"

	// Prime the monitor with initial state
	m.lastHeight = 99
	m.lastCheckTime = time.Now().Add(-1 * time.Second) // 1 second ago
	m.staleCount = 2                                   // already stale

	// Check with height 100 - that's 1 block in 1 second = 1 bl/sec (slow, just following)
	result, err := m.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CheckResultStaleWarning, result)
	assert.Equal(t, 3, m.StaleCount(), "should NOT reset stale count when making slow progress")
}

func TestSyncMonitor_StaleAndStuck_Warning(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")
	m.WarnThreshold = 3
	m.ReconnectThreshold = 5

	// Prime state - node is at height 100, not making any progress
	m.lastHeight = 100
	m.lastCheckTime = time.Now().Add(-1 * time.Second)

	// Now checks with same height are stuck (no progress)
	for i := 0; i < 3; i++ {
		result, err := m.Check(context.Background())
		require.NoError(t, err)
		assert.Equal(t, CheckResultStaleWarning, result)
	}

	assert.Equal(t, 3, m.StaleCount())
	assert.Empty(t, dialer.dialCalls, "should not dial before reconnect threshold")
}

func TestSyncMonitor_StaleAndStuck_Reconnect(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656,peer2@localhost:26657")
	m.WarnThreshold = 2
	m.ReconnectThreshold = 4

	// Prime state - node is at height 100, not making any progress
	m.lastHeight = 100
	m.lastCheckTime = time.Now().Add(-1 * time.Second)

	// Simulate being stuck for reconnect threshold
	for i := 0; i < 4; i++ {
		result, err := m.Check(context.Background())
		require.NoError(t, err)

		if i < 3 {
			assert.Equal(t, CheckResultStaleWarning, result)
		} else {
			assert.Equal(t, CheckResultStaleReconnect, result)
		}
	}

	require.Len(t, dialer.dialCalls, 1, "should have dialed once")
	assert.Equal(t, []string{"peer1@localhost:26656", "peer2@localhost:26657"}, dialer.dialCalls[0])
	assert.Equal(t, 0, m.StaleCount(), "should reset after reconnect")
}

func TestSyncMonitor_StatusError(t *testing.T) {
	status := &mockStatusProvider{
		err: errors.New("connection refused"),
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	result, err := m.Check(context.Background())
	assert.Error(t, err)
	assert.Equal(t, CheckResultCatchingUp, result) // returns catching up on error
}

func TestSyncMonitor_NoPersistentPeers(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{}

	// No persistent peers configured
	m := NewSyncMonitor(status, dialer, "")
	m.ReconnectThreshold = 2

	// Prime state - node is at height 100, not making any progress
	m.lastHeight = 100
	m.lastCheckTime = time.Now().Add(-1 * time.Second)

	// Trigger reconnect threshold
	_, _ = m.Check(context.Background())
	_, _ = m.Check(context.Background())

	assert.Empty(t, dialer.dialCalls, "should not dial when no peers configured")
}

func TestSyncMonitor_DialError(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{
		err: errors.New("dial failed"),
	}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")
	m.ReconnectThreshold = 2

	// Prime state - node is at height 100, not making any progress
	m.lastHeight = 100
	m.lastCheckTime = time.Now().Add(-1 * time.Second)

	// Trigger reconnect - should not panic on dial error
	_, _ = m.Check(context.Background())
	result, err := m.Check(context.Background())

	require.NoError(t, err) // Check itself shouldn't error
	assert.Equal(t, CheckResultStaleReconnect, result)
	assert.Len(t, dialer.dialCalls, 1, "should have attempted dial")
}

func TestSyncMonitor_TransitionFromCatchingUpToSynced(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        true,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Hour),
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	// Initially catching up
	result, _ := m.Check(context.Background())
	assert.Equal(t, CheckResultCatchingUp, result)

	// Transition to synced
	status.status.CatchingUp = false
	status.status.LatestBlockTime = time.Now()

	result, _ = m.Check(context.Background())
	assert.Equal(t, CheckResultSynced, result)
}

func TestSyncMonitor_Reset(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute),
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	// Build up some state
	m.lastHeight = 100
	m.lastCheckTime = time.Now().Add(-1 * time.Second)
	_, _ = m.Check(context.Background())
	_, _ = m.Check(context.Background())
	assert.Greater(t, m.StaleCount(), 0)

	// Reset
	m.Reset()
	assert.Equal(t, 0, m.StaleCount())
	assert.Equal(t, int64(0), m.lastHeight)
	assert.True(t, m.lastCheckTime.IsZero())
}
