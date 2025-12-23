// Copyright 2025 The Accumulate Authors
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

func TestSyncMonitor_StaleButMakingProgress(t *testing.T) {
	status := &mockStatusProvider{
		status: &SyncStatus{
			CatchingUp:        false,
			LatestBlockHeight: 100,
			LatestBlockTime:   time.Now().Add(-1 * time.Minute), // stale
		},
	}
	dialer := &mockPeerDialer{}

	m := NewSyncMonitor(status, dialer, "peer1@localhost:26656")

	// First check - stale but first time seeing this height
	result, err := m.Check(context.Background())
	require.NoError(t, err)
	// First check won't show progress since lastHeight was 0
	assert.Equal(t, CheckResultStaleProgress, result)

	// Second check with increased height - making progress
	status.status.LatestBlockHeight = 101
	result, err = m.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CheckResultStaleProgress, result)
	assert.Equal(t, 0, m.StaleCount(), "should reset stale count when making progress")
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

	// First check sets lastHeight, but since height > 0 (lastHeight), it shows progress
	result, _ := m.Check(context.Background())
	assert.Equal(t, CheckResultStaleProgress, result)
	assert.Equal(t, 0, m.StaleCount())

	// Now subsequent checks with same height are stuck
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

	// First check sets lastHeight
	m.Check(context.Background())

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

	// First check
	m.Check(context.Background())

	// Trigger reconnect threshold
	m.Check(context.Background())
	m.Check(context.Background())

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

	// First check
	m.Check(context.Background())

	// Trigger reconnect - should not panic on dial error
	m.Check(context.Background())
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
	m.Check(context.Background())
	m.Check(context.Background())
	assert.Greater(t, m.StaleCount(), 0)

	// Reset
	m.Reset()
	assert.Equal(t, 0, m.StaleCount())
}
