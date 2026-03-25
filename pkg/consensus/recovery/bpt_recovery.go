// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package recovery provides BPT-level sync recovery for DAG-BFT consensus.
// It coordinates the recovery process when state divergence is detected,
// including requesting missing BPT entries, validation passes, and chain
// entry gap filling.
package recovery

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/primary"
)

// RecoveryState represents the current state of BPT recovery.
type RecoveryState int

const (
	// RecoveryStateIdle means no recovery is in progress.
	RecoveryStateIdle RecoveryState = iota

	// RecoveryStateDetecting means divergence has been detected and we're
	// determining what needs to be recovered.
	RecoveryStateDetecting

	// RecoveryStateSyncing means we're actively syncing missing BPT entries.
	RecoveryStateSyncing

	// RecoveryStateValidating means we're running validation passes.
	RecoveryStateValidating

	// RecoveryStateFillingChains means we're filling chain entry gaps.
	RecoveryStateFillingChains

	// RecoveryStateComplete means recovery has completed successfully.
	RecoveryStateComplete

	// RecoveryStateFailed means recovery has failed.
	RecoveryStateFailed
)

func (s RecoveryState) String() string {
	switch s {
	case RecoveryStateIdle:
		return "idle"
	case RecoveryStateDetecting:
		return "detecting"
	case RecoveryStateSyncing:
		return "syncing"
	case RecoveryStateValidating:
		return "validating"
	case RecoveryStateFillingChains:
		return "filling_chains"
	case RecoveryStateComplete:
		return "complete"
	case RecoveryStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// BPTRecoveryConfig holds configuration for BPT recovery.
type BPTRecoveryConfig struct {
	// SyncerConfig configures the BPT syncer.
	SyncerConfig primary.BPTSyncerConfig

	// WalkerConfig configures the BPT walker.
	WalkerConfig primary.BPTWalkerConfig

	// ValidatorConfig configures the BPT validator.
	ValidatorConfig primary.BPTValidatorConfig

	// ChainFillerConfig configures the chain gap filler.
	ChainFillerConfig primary.ChainGapFillerConfig

	// RecoveryTimeout is the maximum time to spend on recovery before failing.
	RecoveryTimeout time.Duration
}

// applyDefaults fills in default values for unset configuration fields.
func (c *BPTRecoveryConfig) applyDefaults() {
	// Note: The sub-component configs will have their defaults applied
	// in their respective constructors (NewBPTSyncer, NewBPTWalker, etc.)

	if c.RecoveryTimeout <= 0 {
		c.RecoveryTimeout = 5 * time.Minute
	}
}

// BPTRecoveryProvider provides access to BPT and chain data for recovery.
// This interface combines the requirements of all sub-components.
type BPTRecoveryProvider interface {
	primary.BPTStore
	primary.BPTHashProvider
	primary.BPTValidationProvider
	primary.ChainEntryProvider
}

// BPTRecoveryCoordinator coordinates the BPT sync recovery process.
// It manages the lifecycle of the syncer, walker, validator, and chain filler.
type BPTRecoveryCoordinator struct {
	config BPTRecoveryConfig

	// Sub-components
	syncer      *primary.BPTSyncer
	walker      *primary.BPTWalker
	validator   *primary.BPTValidator
	chainFiller *primary.ChainGapFiller

	// State tracking
	state        atomic.Int32 // RecoveryState
	startTime    time.Time
	failureCount atomic.Int32

	// Callbacks
	onComplete func()
	onFailed   func(reason string)

	// Lifecycle management
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBPTRecoveryCoordinator creates a new BPT recovery coordinator.
func NewBPTRecoveryCoordinator(
	config BPTRecoveryConfig,
	provider BPTRecoveryProvider,
	gossipLayer *gossip.GossipLayer,
	chainRequester primary.ChainEntryRequester,
) *BPTRecoveryCoordinator {
	config.applyDefaults()

	// Create sub-components
	syncer := primary.NewBPTSyncer(config.SyncerConfig, provider, gossipLayer)
	walker := primary.NewBPTWalker(config.WalkerConfig, provider, syncer)
	validator := primary.NewBPTValidator(config.ValidatorConfig, provider, walker)
	chainFiller := primary.NewChainGapFiller(config.ChainFillerConfig, provider, chainRequester)

	coord := &BPTRecoveryCoordinator{
		config:      config,
		syncer:      syncer,
		walker:      walker,
		validator:   validator,
		chainFiller: chainFiller,
	}

	// Wire up callbacks
	syncer.SetEntryReceivedCallback(func(keyHash [32]byte, value []byte) {
		walker.OnEntryReceived(keyHash)
	})

	validator.SetCallbacks(
		func() { coord.onValidationConverged() },
		func(reason string) { coord.onValidationFailed(reason) },
	)

	return coord
}

// SetCallbacks sets the callbacks for recovery events.
func (c *BPTRecoveryCoordinator) SetCallbacks(onComplete func(), onFailed func(reason string)) {
	c.onComplete = onComplete
	c.onFailed = onFailed
}

// State returns the current recovery state.
func (c *BPTRecoveryCoordinator) State() RecoveryState {
	return RecoveryState(c.state.Load())
}

// Start begins the recovery process.
func (c *BPTRecoveryCoordinator) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.ctx != nil {
		c.mu.Unlock()
		return nil
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.startTime = time.Now()
	c.mu.Unlock()

	c.state.Store(int32(RecoveryStateDetecting))

	// Start sub-components
	if err := c.syncer.Start(c.ctx); err != nil {
		return err
	}
	if err := c.walker.Start(c.ctx); err != nil {
		return err
	}
	if err := c.validator.Start(c.ctx); err != nil {
		return err
	}
	if err := c.chainFiller.Start(c.ctx); err != nil {
		return err
	}

	// Start the recovery monitor
	c.wg.Add(1)
	go c.monitorRecovery()

	slog.Info("BPTRecoveryCoordinator started")
	c.state.Store(int32(RecoveryStateSyncing))
	return nil
}

// Stop stops the recovery process.
func (c *BPTRecoveryCoordinator) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()

	c.wg.Wait()

	c.chainFiller.Stop()
	c.validator.Stop()
	c.walker.Stop()
	c.syncer.Stop()

	slog.Info("BPTRecoveryCoordinator stopped")
}

// monitorRecovery monitors the recovery process and handles timeouts.
func (c *BPTRecoveryCoordinator) monitorRecovery() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return

		case <-ticker.C:
			// Check for timeout
			if time.Since(c.startTime) > c.config.RecoveryTimeout {
				c.onRecoveryTimeout()
				return
			}

			// Update state based on sub-component states
			c.updateState()
		}
	}
}

// updateState updates the recovery state based on sub-component states.
func (c *BPTRecoveryCoordinator) updateState() {
	state := RecoveryState(c.state.Load())

	switch state {
	case RecoveryStateSyncing:
		// Check if walker has resolved divergence
		if !c.walker.IsDiverged() && c.walker.PendingGapCount() == 0 {
			c.state.Store(int32(RecoveryStateValidating))
			slog.Info("BPTRecovery: transitioning to validation")
		}

	case RecoveryStateValidating:
		// Validator handles its own state transitions via callbacks
		if c.validator.State() == primary.ValidationStateConverged {
			c.state.Store(int32(RecoveryStateFillingChains))
			slog.Info("BPTRecovery: transitioning to chain filling")
		}

	case RecoveryStateFillingChains:
		// Check if chain filler has completed
		if c.chainFiller.PendingGapCount() == 0 {
			c.onRecoveryComplete()
		}
	}
}

// onValidationConverged is called when BPT validation converges.
func (c *BPTRecoveryCoordinator) onValidationConverged() {
	slog.Info("BPTRecovery: validation converged")
	c.state.Store(int32(RecoveryStateFillingChains))

	// Trigger chain gap check
	c.chainFiller.TriggerCheck()
}

// onValidationFailed is called when BPT validation fails.
func (c *BPTRecoveryCoordinator) onValidationFailed(reason string) {
	slog.Error("BPTRecovery: validation failed", "reason", reason)
	c.failureCount.Add(1)

	// If we've failed too many times, give up
	if c.failureCount.Load() >= 3 {
		c.state.Store(int32(RecoveryStateFailed))
		if c.onFailed != nil {
			c.onFailed("validation failed after multiple attempts: " + reason)
		}
		return
	}

	// Otherwise, restart syncing
	c.state.Store(int32(RecoveryStateSyncing))
	c.validator.Reset()
	c.walker.TriggerWalk()
}

// onRecoveryComplete is called when recovery completes successfully.
func (c *BPTRecoveryCoordinator) onRecoveryComplete() {
	c.state.Store(int32(RecoveryStateComplete))
	slog.Info("BPTRecovery: recovery complete",
		"duration", time.Since(c.startTime))

	if c.onComplete != nil {
		c.onComplete()
	}
}

// onRecoveryTimeout is called when recovery times out.
func (c *BPTRecoveryCoordinator) onRecoveryTimeout() {
	c.state.Store(int32(RecoveryStateFailed))
	slog.Error("BPTRecovery: recovery timeout",
		"timeout", c.config.RecoveryTimeout,
		"state", c.State().String())

	if c.onFailed != nil {
		c.onFailed("recovery timeout")
	}
}

// Status returns the current recovery status.
type RecoveryStatus struct {
	State           RecoveryState
	StartTime       time.Time
	Duration        time.Duration
	PendingBPTGaps  int
	PendingChainGaps int
	FailureCount    int
	ValidatorState  primary.ValidationState
}

// Status returns the current recovery status.
func (c *BPTRecoveryCoordinator) Status() RecoveryStatus {
	return RecoveryStatus{
		State:            c.State(),
		StartTime:        c.startTime,
		Duration:         time.Since(c.startTime),
		PendingBPTGaps:   c.walker.PendingGapCount(),
		PendingChainGaps: c.chainFiller.PendingGapCount(),
		FailureCount:     int(c.failureCount.Load()),
		ValidatorState:   c.validator.State(),
	}
}

// Metrics returns recovery metrics.
func (c *BPTRecoveryCoordinator) Metrics() (syncerMetrics, walkerMetrics, validatorMetrics, chainFillerMetrics [3]uint64) {
	sr, srsp, se := c.syncer.Metrics()
	syncerMetrics = [3]uint64{sr, srsp, se}

	wc, wf, wr := c.walker.Metrics()
	walkerMetrics = [3]uint64{wc, wf, wr}

	vt, vs, vf := c.validator.Metrics()
	validatorMetrics = [3]uint64{vt, vs, vf}

	cgd, cgf, ce := c.chainFiller.Metrics()
	chainFillerMetrics = [3]uint64{cgd, cgf, ce}

	return
}
