// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Default configuration values for BPT validation.
const (
	// DefaultBPTValidationInterval is the interval between validation passes.
	DefaultBPTValidationInterval = 2 * time.Second

	// DefaultBPTValidationMaxPasses is the maximum number of validation passes
	// before escalating to a full resync.
	DefaultBPTValidationMaxPasses = 10

	// DefaultBPTValidationConvergenceThreshold is the number of consecutive
	// successful validations required to consider state converged.
	DefaultBPTValidationConvergenceThreshold = 3
)

// ValidationState represents the current state of BPT validation.
type ValidationState int

const (
	// ValidationStateIdle means no validation is in progress.
	ValidationStateIdle ValidationState = iota

	// ValidationStateRunning means validation is actively running.
	ValidationStateRunning

	// ValidationStateConverged means state has successfully converged.
	ValidationStateConverged

	// ValidationStateFailed means validation has failed after max passes.
	ValidationStateFailed
)

func (s ValidationState) String() string {
	switch s {
	case ValidationStateIdle:
		return "idle"
	case ValidationStateRunning:
		return "running"
	case ValidationStateConverged:
		return "converged"
	case ValidationStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// BPTValidatorConfig holds configuration for the BPTValidator.
type BPTValidatorConfig struct {
	// ValidationInterval is the interval between validation passes.
	ValidationInterval time.Duration

	// MaxPasses is the maximum number of validation passes before escalating.
	MaxPasses int

	// ConvergenceThreshold is the number of consecutive successful validations
	// required to consider state converged.
	ConvergenceThreshold int
}

// applyDefaults fills in default values for unset configuration fields.
func (c *BPTValidatorConfig) applyDefaults() {
	if c.ValidationInterval <= 0 {
		c.ValidationInterval = DefaultBPTValidationInterval
	}
	if c.MaxPasses <= 0 {
		c.MaxPasses = DefaultBPTValidationMaxPasses
	}
	if c.ConvergenceThreshold <= 0 {
		c.ConvergenceThreshold = DefaultBPTValidationConvergenceThreshold
	}
}

// BPTValidationProvider provides BPT state for validation.
type BPTValidationProvider interface {
	// GetExpectedRootHash returns the expected root hash.
	GetExpectedRootHash() *[32]byte

	// GetLocalRootHash returns the local BPT root hash.
	GetLocalRootHash() ([32]byte, error)

	// RecalculateHashes forces recalculation of all BPT hashes.
	// This should be called after BPT entries are updated to ensure
	// the hash tree is consistent.
	RecalculateHashes() error
}

// BPTValidator coordinates validation passes for BPT synchronization.
// It tracks convergence progress and escalates when needed.
type BPTValidator struct {
	config   BPTValidatorConfig
	provider BPTValidationProvider
	walker   *BPTWalker

	// State tracking
	state               atomic.Int32 // ValidationState
	passCount           atomic.Int32
	consecutiveSuccess  atomic.Int32
	invalidNodesCount   atomic.Int32
	lastInvalidCount    atomic.Int32
	unchangedPassCount  atomic.Int32

	// Callbacks
	onConverged func()
	onFailed    func(reason string)

	// Lifecycle management
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	totalPasses       atomic.Uint64
	successfulPasses  atomic.Uint64
	failedPasses      atomic.Uint64
	lastValidationErr atomic.Value // *string or nil
}

// NewBPTValidator creates a new BPTValidator.
func NewBPTValidator(
	config BPTValidatorConfig,
	provider BPTValidationProvider,
	walker *BPTWalker,
) *BPTValidator {
	config.applyDefaults()

	return &BPTValidator{
		config:   config,
		provider: provider,
		walker:   walker,
	}
}

// SetCallbacks sets the callbacks for validation events.
func (v *BPTValidator) SetCallbacks(onConverged func(), onFailed func(reason string)) {
	v.onConverged = onConverged
	v.onFailed = onFailed
}

// Start begins the validation loop.
func (v *BPTValidator) Start(ctx context.Context) error {
	v.mu.Lock()
	if v.ctx != nil {
		v.mu.Unlock()
		return nil
	}
	v.ctx, v.cancel = context.WithCancel(ctx)
	v.mu.Unlock()

	v.wg.Add(1)
	go v.validationLoop()

	slog.Info("BPTValidator started")
	return nil
}

// Stop stops the validation loop.
func (v *BPTValidator) Stop() {
	v.mu.Lock()
	if v.cancel != nil {
		v.cancel()
	}
	v.mu.Unlock()

	v.wg.Wait()
	slog.Info("BPTValidator stopped",
		"totalPasses", v.totalPasses.Load(),
		"successfulPasses", v.successfulPasses.Load(),
		"failedPasses", v.failedPasses.Load())
}

// State returns the current validation state.
func (v *BPTValidator) State() ValidationState {
	return ValidationState(v.state.Load())
}

// PassCount returns the number of validation passes performed.
func (v *BPTValidator) PassCount() int {
	return int(v.passCount.Load())
}

// InvalidNodesCount returns the current count of invalid nodes.
func (v *BPTValidator) InvalidNodesCount() int {
	return int(v.invalidNodesCount.Load())
}

// TriggerValidation triggers an immediate validation pass.
func (v *BPTValidator) TriggerValidation() {
	go v.runValidationPass()
}

// Reset resets the validator state for a new sync cycle.
func (v *BPTValidator) Reset() {
	v.state.Store(int32(ValidationStateIdle))
	v.passCount.Store(0)
	v.consecutiveSuccess.Store(0)
	v.invalidNodesCount.Store(0)
	v.lastInvalidCount.Store(0)
	v.unchangedPassCount.Store(0)
}

// validationLoop is the main validation loop.
func (v *BPTValidator) validationLoop() {
	defer v.wg.Done()

	ticker := time.NewTicker(v.config.ValidationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-v.ctx.Done():
			return

		case <-ticker.C:
			// Only run validation if we're in a diverged state
			if v.walker != nil && v.walker.IsDiverged() {
				v.runValidationPass()
			}
		}
	}
}

// runValidationPass runs a single validation pass.
func (v *BPTValidator) runValidationPass() {
	v.state.Store(int32(ValidationStateRunning))
	v.totalPasses.Add(1)
	passNum := v.passCount.Add(1)

	slog.Debug("BPTValidator: starting validation pass", "pass", passNum)

	// Recalculate hashes to ensure consistency
	if v.provider != nil {
		if err := v.provider.RecalculateHashes(); err != nil {
			slog.Warn("BPTValidator: failed to recalculate hashes", "error", err)
			errStr := err.Error()
			v.lastValidationErr.Store(&errStr)
			v.failedPasses.Add(1)
			return
		}
	}

	// Get expected root hash
	expectedRoot := v.provider.GetExpectedRootHash()
	if expectedRoot == nil {
		slog.Debug("BPTValidator: no expected root hash")
		return
	}

	// Get local root hash
	localRoot, err := v.provider.GetLocalRootHash()
	if err != nil {
		slog.Warn("BPTValidator: failed to get local root hash", "error", err)
		errStr := err.Error()
		v.lastValidationErr.Store(&errStr)
		v.failedPasses.Add(1)
		return
	}

	// Compare hashes
	if localRoot == *expectedRoot {
		// State is valid
		v.successfulPasses.Add(1)
		v.invalidNodesCount.Store(0)

		consecutiveSuccess := v.consecutiveSuccess.Add(1)
		slog.Debug("BPTValidator: pass successful",
			"pass", passNum,
			"consecutiveSuccess", consecutiveSuccess)

		// Check if we've reached convergence
		if consecutiveSuccess >= int32(v.config.ConvergenceThreshold) {
			v.onConvergence()
		}
		return
	}

	// State is still invalid
	v.consecutiveSuccess.Store(0)

	// Check if the number of invalid nodes changed
	currentInvalid := v.invalidNodesCount.Load()
	lastInvalid := v.lastInvalidCount.Swap(currentInvalid)

	if currentInvalid == lastInvalid && currentInvalid > 0 {
		// No progress made
		unchangedCount := v.unchangedPassCount.Add(1)
		slog.Warn("BPTValidator: no progress made",
			"pass", passNum,
			"invalidNodes", currentInvalid,
			"unchangedPasses", unchangedCount)

		// Check for max passes exceeded
		if int(passNum) >= v.config.MaxPasses {
			v.onFailure("max validation passes exceeded without convergence")
			return
		}
	} else {
		// Progress was made
		v.unchangedPassCount.Store(0)
		slog.Debug("BPTValidator: progress made",
			"pass", passNum,
			"previousInvalid", lastInvalid,
			"currentInvalid", currentInvalid)
	}

	// Trigger another sync attempt
	if v.walker != nil {
		v.walker.TriggerWalk()
	}
}

// onConvergence is called when state has converged.
func (v *BPTValidator) onConvergence() {
	v.state.Store(int32(ValidationStateConverged))
	slog.Info("BPTValidator: state converged",
		"passes", v.passCount.Load())

	if v.onConverged != nil {
		v.onConverged()
	}
}

// onFailure is called when validation has failed.
func (v *BPTValidator) onFailure(reason string) {
	v.state.Store(int32(ValidationStateFailed))
	v.failedPasses.Add(1)
	slog.Error("BPTValidator: validation failed",
		"reason", reason,
		"passes", v.passCount.Load())

	if v.onFailed != nil {
		v.onFailed(reason)
	}
}

// Metrics returns validator metrics.
func (v *BPTValidator) Metrics() (totalPasses, successfulPasses, failedPasses uint64) {
	return v.totalPasses.Load(), v.successfulPasses.Load(), v.failedPasses.Load()
}

// LastValidationError returns the last validation error, if any.
func (v *BPTValidator) LastValidationError() string {
	if e := v.lastValidationErr.Load(); e != nil {
		return *e.(*string)
	}
	return ""
}
