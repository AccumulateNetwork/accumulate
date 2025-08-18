// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build testnet
// +build testnet

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPauseMechanism tests the basic pause/resume functionality
func TestPauseMechanism(t *testing.T) {
	// Create a minimal conductor for testing
	cc := &CrossChainConductor{}

	// Test initial state
	require.False(t, cc.IsPaused(), "Should not be paused initially")

	// Test pause
	cc.Pause()
	require.True(t, cc.IsPaused(), "Should be paused after Pause()")

	// Test resume
	cc.Resume()
	require.False(t, cc.IsPaused(), "Should not be paused after Resume()")

	// Test multiple pause/resume
	cc.Pause()
	cc.Pause() // Should be idempotent
	require.True(t, cc.IsPaused(), "Should remain paused")

	cc.Resume()
	cc.Resume() // Should be idempotent
	require.False(t, cc.IsPaused(), "Should remain resumed")
}
