// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestHaltControllerIntegration tests that the HaltController correctly
// triggers shutdown when a major block is committed after halt is requested.
func TestHaltControllerIntegration(t *testing.T) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	// Initialize with a short major block schedule for faster testing
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "*/5 * * * *" // Every 5 minutes (300 minor blocks)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Track shutdown calls for each partition
	var dnShutdownCalled atomic.Bool
	var bvn0ShutdownCalled atomic.Bool
	var bvn1ShutdownCalled atomic.Bool
	var bvn2ShutdownCalled atomic.Bool

	// Create halt controllers for each partition and subscribe them to the event bus
	dnHalt := run.NewHaltController(Directory, func() { dnShutdownCalled.Store(true) }, slog.Default())
	bvn0Halt := run.NewHaltController("BVN0", func() { bvn0ShutdownCalled.Store(true) }, slog.Default())
	bvn1Halt := run.NewHaltController("BVN1", func() { bvn1ShutdownCalled.Store(true) }, slog.Default())
	bvn2Halt := run.NewHaltController("BVN2", func() { bvn2ShutdownCalled.Store(true) }, slog.Default())

	// Subscribe halt controllers to the event buses
	events.SubscribeSync(sim.S.EventBus(Directory), dnHalt.OnDidCommitBlock)
	events.SubscribeSync(sim.S.EventBus("BVN0"), bvn0Halt.OnDidCommitBlock)
	events.SubscribeSync(sim.S.EventBus("BVN1"), bvn1Halt.OnDidCommitBlock)
	events.SubscribeSync(sim.S.EventBus("BVN2"), bvn2Halt.OnDidCommitBlock)

	// Calculate when the major block will occur
	nextMajorBlock := g.MajorBlockSchedule().Next(GenesisTime)
	blocksUntilMajor := int(nextMajorBlock.Sub(GenesisTime) / time.Second)

	// Step to just before the major block (matching pattern from net_major_block_test.go)
	sim.StepN(blocksUntilMajor - GenesisBlock - 1)

	// Verify no shutdown yet (no halt requested, and no major block yet)
	require.False(t, dnShutdownCalled.Load(), "DN shutdown should not be called before halt requested")
	require.False(t, bvn0ShutdownCalled.Load(), "BVN0 shutdown should not be called before halt requested")

	// Request halt on all partitions
	dnHalt.RequestHalt()
	bvn0Halt.RequestHalt()
	bvn1Halt.RequestHalt()
	bvn2Halt.RequestHalt()

	// Verify halt is pending
	require.True(t, dnHalt.IsHaltPending(), "DN halt should be pending")
	require.True(t, bvn0Halt.IsHaltPending(), "BVN0 halt should be pending")

	// Verify no shutdown yet (halt requested but no major block yet)
	require.False(t, dnShutdownCalled.Load(), "DN shutdown should not be called before major block")

	// Execute the major block on DN - this opens the major block
	sim.Step()

	// Execute more blocks to complete the major block and propagate to BVNs
	sim.StepN(10)

	// Give the async shutdown goroutine time to run
	time.Sleep(100 * time.Millisecond)

	// DN should have triggered shutdown after major block
	require.True(t, dnShutdownCalled.Load(), "DN shutdown should be called after major block when halt requested")

	// All BVNs should have triggered shutdown
	require.True(t, bvn0ShutdownCalled.Load(), "BVN0 shutdown should be called after major block")
	require.True(t, bvn1ShutdownCalled.Load(), "BVN1 shutdown should be called after major block")
	require.True(t, bvn2ShutdownCalled.Load(), "BVN2 shutdown should be called after major block")
}

// TestHaltControllerNoShutdownWithoutRequest verifies that a major block does
// NOT trigger shutdown if halt was not requested.
func TestHaltControllerNoShutdownWithoutRequest(t *testing.T) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "*/5 * * * *"
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Track shutdown calls
	var shutdownCalled atomic.Bool

	// Create halt controller but do NOT request halt
	haltController := run.NewHaltController(Directory, func() { shutdownCalled.Store(true) }, slog.Default())
	events.SubscribeSync(sim.S.EventBus(Directory), haltController.OnDidCommitBlock)

	// Calculate when major block occurs and step past it
	nextMajorBlock := g.MajorBlockSchedule().Next(GenesisTime)
	blocksUntilMajor := int(nextMajorBlock.Sub(GenesisTime) / time.Second)

	// Execute past the major block
	sim.StepN(blocksUntilMajor + 10)
	time.Sleep(100 * time.Millisecond)

	// Verify major block was actually created
	ledger := GetAccount[*AnchorLedger](t, sim.Database(Directory), DnUrl().JoinPath(AnchorPool))
	require.Empty(t, ledger.PendingMajorBlockAnchors, "Major block should have completed")

	// Shutdown should NOT have been called since halt was not requested
	require.False(t, shutdownCalled.Load(), "Shutdown should not be called when halt was not requested")
}

// TestHaltControllerCancelHalt verifies that canceling a halt request prevents
// shutdown even after a major block.
func TestHaltControllerCancelHalt(t *testing.T) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "*/5 * * * *"
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Track shutdown calls
	var shutdownCalled atomic.Bool

	// Create halt controller
	haltController := run.NewHaltController(Directory, func() { shutdownCalled.Store(true) }, slog.Default())
	events.SubscribeSync(sim.S.EventBus(Directory), haltController.OnDidCommitBlock)

	// Calculate timing
	nextMajorBlock := g.MajorBlockSchedule().Next(GenesisTime)
	blocksUntilMajor := int(nextMajorBlock.Sub(GenesisTime) / time.Second)

	// Step to just before major block
	sim.StepN(blocksUntilMajor - GenesisBlock - 5)

	// Request halt, then cancel it
	haltController.RequestHalt()
	require.True(t, haltController.IsHaltPending())
	haltController.CancelHalt()
	require.False(t, haltController.IsHaltPending())

	// Execute past the major block
	sim.StepN(20)
	time.Sleep(100 * time.Millisecond)

	// Shutdown should NOT have been called since halt was canceled
	require.False(t, shutdownCalled.Load(), "Shutdown should not be called when halt was canceled")
}
