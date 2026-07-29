// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
)

func TestHaltController(t *testing.T) {
	t.Run("RequestHalt sets flag", func(t *testing.T) {
		var shutdownCalled atomic.Bool
		hc := NewHaltController("test", func() { shutdownCalled.Store(true) }, slog.Default())

		require.False(t, hc.IsHaltPending())
		hc.RequestHalt()
		require.True(t, hc.IsHaltPending())
	})

	t.Run("CancelHalt clears flag", func(t *testing.T) {
		var shutdownCalled atomic.Bool
		hc := NewHaltController("test", func() { shutdownCalled.Store(true) }, slog.Default())

		hc.RequestHalt()
		require.True(t, hc.IsHaltPending())
		hc.CancelHalt()
		require.False(t, hc.IsHaltPending())
	})

	t.Run("Shutdown on major block when halt requested", func(t *testing.T) {
		var shutdownCalled atomic.Bool
		hc := NewHaltController("test", func() { shutdownCalled.Store(true) }, slog.Default())

		// Request halt
		hc.RequestHalt()

		// Simulate minor block - should not trigger shutdown
		err := hc.OnDidCommitBlock(events.DidCommitBlock{
			Index: 100,
			Time:  time.Now(),
			Major: 0, // Not a major block
		})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Give goroutine time to run
		require.False(t, shutdownCalled.Load(), "shutdown should not be called on minor block")

		// Simulate major block - should trigger shutdown
		err = hc.OnDidCommitBlock(events.DidCommitBlock{
			Index: 101,
			Time:  time.Now(),
			Major: 5, // Major block
		})
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond) // Give goroutine time to run
		require.True(t, shutdownCalled.Load(), "shutdown should be called on major block")
	})

	t.Run("No shutdown on major block when halt not requested", func(t *testing.T) {
		var shutdownCalled atomic.Bool
		hc := NewHaltController("test", func() { shutdownCalled.Store(true) }, slog.Default())

		// Do NOT request halt

		// Simulate major block - should NOT trigger shutdown
		err := hc.OnDidCommitBlock(events.DidCommitBlock{
			Index: 100,
			Time:  time.Now(),
			Major: 5, // Major block
		})
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond) // Give goroutine time to run
		require.False(t, shutdownCalled.Load(), "shutdown should not be called when halt not requested")
	})

	t.Run("Partition returns correct value", func(t *testing.T) {
		hc := NewHaltController("Directory", func() {}, slog.Default())
		require.Equal(t, "Directory", hc.Partition())
	})
}

func TestInstanceHaltMethods(t *testing.T) {
	t.Run("RegisterHaltController", func(t *testing.T) {
		inst := &Instance{}
		hc1 := NewHaltController("Directory", func() {}, slog.Default())
		hc2 := NewHaltController("BVN0", func() {}, slog.Default())

		inst.RegisterHaltController(hc1)
		inst.RegisterHaltController(hc2)

		status := inst.GetHaltStatus()
		require.False(t, status.Pending)
		require.Empty(t, status.Partitions)
	})

	t.Run("RequestHaltAll and GetHaltStatus", func(t *testing.T) {
		inst := &Instance{}
		hc1 := NewHaltController("Directory", func() {}, slog.Default())
		hc2 := NewHaltController("BVN0", func() {}, slog.Default())

		inst.RegisterHaltController(hc1)
		inst.RegisterHaltController(hc2)

		inst.RequestHaltAll()

		status := inst.GetHaltStatus()
		require.True(t, status.Pending)
		require.Len(t, status.Partitions, 2)
		require.Contains(t, status.Partitions, "Directory")
		require.Contains(t, status.Partitions, "BVN0")
	})

	t.Run("CancelHaltAll", func(t *testing.T) {
		inst := &Instance{}
		hc1 := NewHaltController("Directory", func() {}, slog.Default())
		hc2 := NewHaltController("BVN0", func() {}, slog.Default())

		inst.RegisterHaltController(hc1)
		inst.RegisterHaltController(hc2)

		inst.RequestHaltAll()
		require.True(t, inst.GetHaltStatus().Pending)

		inst.CancelHaltAll()
		require.False(t, inst.GetHaltStatus().Pending)
	})
}
