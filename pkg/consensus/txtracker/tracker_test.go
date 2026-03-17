// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package txtracker

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracker_Track(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	// Verify hash matches
	expected := sha256.Sum256(tx)
	assert.Equal(t, expected, hash)

	// Verify status is pending
	status, err := tracker.GetStatus(hash)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, status)
}

func TestTracker_SetStatus(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	// Status transitions
	tracker.SetStatus(hash, StatusBatched)
	status, err := tracker.GetStatus(hash)
	require.NoError(t, err)
	assert.Equal(t, StatusBatched, status)

	tracker.SetStatus(hash, StatusCertified)
	status, err = tracker.GetStatus(hash)
	require.NoError(t, err)
	assert.Equal(t, StatusCertified, status)

	tracker.SetStatus(hash, StatusCommitted)
	status, err = tracker.GetStatus(hash)
	require.NoError(t, err)
	assert.Equal(t, StatusCommitted, status)
}

func TestTracker_StatusDowngradeNotAllowed(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	// Set to committed
	tracker.SetStatus(hash, StatusCommitted)

	// Try to downgrade (should be ignored)
	tracker.SetStatus(hash, StatusBatched)

	status, err := tracker.GetStatus(hash)
	require.NoError(t, err)
	assert.Equal(t, StatusCommitted, status) // Still committed
}

func TestTracker_FailedOverridesAny(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	// Set to committed
	tracker.SetStatus(hash, StatusCommitted)

	// Failed can override
	tracker.SetStatus(hash, StatusFailed)

	status, err := tracker.GetStatus(hash)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, status)
}

func TestTracker_WaitForStatus(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	// Wait in goroutine
	done := make(chan error)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done <- tracker.WaitForStatus(ctx, hash, StatusCommitted)
	}()

	// Give waiter time to register
	time.Sleep(50 * time.Millisecond)

	// Set statuses
	tracker.SetStatus(hash, StatusBatched)
	tracker.SetStatus(hash, StatusCertified)
	tracker.SetStatus(hash, StatusCommitted)

	// Wait should complete
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForStatus timed out")
	}
}

func TestTracker_WaitForStatus_AlreadyReached(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	tracker.SetStatus(hash, StatusCommitted)

	// Wait should return immediately
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := tracker.WaitForStatus(ctx, hash, StatusBatched)
	require.NoError(t, err)
}

func TestTracker_WaitForStatus_Timeout(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := tracker.WaitForStatus(ctx, hash, StatusCommitted)
	assert.Equal(t, ErrTimeout, err)
}

func TestTracker_Eviction(t *testing.T) {
	tracker := NewTracker(TrackerConfig{
		MaxTracked: 3,
	})
	defer tracker.Close()

	// Track 4 transactions
	hashes := make([][32]byte, 4)
	for i := 0; i < 4; i++ {
		tx := []byte{byte(i)}
		hashes[i] = tracker.Track(tx)
	}

	// First transaction should be evicted
	_, err := tracker.GetStatus(hashes[0])
	assert.Equal(t, ErrTransactionNotFound, err)

	// Others should still exist
	for i := 1; i < 4; i++ {
		_, err := tracker.GetStatus(hashes[i])
		require.NoError(t, err)
	}
}

func TestTracker_GetInfo(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	info, err := tracker.GetInfo(hash)
	require.NoError(t, err)
	assert.Equal(t, hash, info.Hash)
	assert.Equal(t, StatusPending, info.Status)
	assert.False(t, info.SubmitTime.IsZero())
}

func TestTracker_MarkBatchIncluded(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	// Track multiple transactions
	hashes := make([][32]byte, 3)
	for i := 0; i < 3; i++ {
		tx := []byte{byte(i)}
		hashes[i] = tracker.Track(tx)
	}

	// Mark all as batched
	tracker.MarkBatchIncluded(hashes)

	for _, hash := range hashes {
		status, err := tracker.GetStatus(hash)
		require.NoError(t, err)
		assert.Equal(t, StatusBatched, status)
	}
}

func TestTracker_MarkCommitted(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})
	defer tracker.Close()

	// Track multiple transactions
	hashes := make([][32]byte, 3)
	for i := 0; i < 3; i++ {
		tx := []byte{byte(i)}
		hashes[i] = tracker.Track(tx)
	}

	// Mark all as committed
	blockIndex := uint64(42)
	tracker.MarkCommitted(hashes, blockIndex)

	for _, hash := range hashes {
		info, err := tracker.GetInfo(hash)
		require.NoError(t, err)
		assert.Equal(t, StatusCommitted, info.Status)
		assert.Equal(t, blockIndex, info.BlockIndex)
	}
}

func TestTracker_Close(t *testing.T) {
	tracker := NewTracker(TrackerConfig{})

	tx := []byte("test transaction")
	hash := tracker.Track(tx)

	// Close should work
	err := tracker.Close()
	require.NoError(t, err)

	// Operations should fail
	_, err = tracker.GetStatus(hash)
	assert.Equal(t, ErrTrackerClosed, err)

	// Double close should be fine
	err = tracker.Close()
	require.NoError(t, err)
}

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUnknown, "unknown"},
		{StatusPending, "pending"},
		{StatusBatched, "batched"},
		{StatusCertified, "certified"},
		{StatusCommitted, "committed"},
		{StatusFailed, "failed"},
		{StatusExpired, "expired"},
		{Status(99), "invalid"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.status.String())
	}
}
