// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBPTValidationProvider is a mock implementation for testing.
type mockBPTValidationProvider struct {
	mu                sync.RWMutex
	expectedRoot      *[32]byte
	localRoot         [32]byte
	localRootErr      error
	recalculateErr    error
	recalculateCalled atomic.Int32
}

func newMockBPTValidationProvider() *mockBPTValidationProvider {
	return &mockBPTValidationProvider{}
}

func (p *mockBPTValidationProvider) GetExpectedRootHash() *[32]byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.expectedRoot
}

func (p *mockBPTValidationProvider) GetLocalRootHash() ([32]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.localRoot, p.localRootErr
}

func (p *mockBPTValidationProvider) RecalculateHashes() error {
	p.recalculateCalled.Add(1)
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.recalculateErr
}

func (p *mockBPTValidationProvider) SetExpectedRoot(root *[32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expectedRoot = root
}

func (p *mockBPTValidationProvider) SetLocalRoot(root [32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.localRoot = root
}

func TestBPTValidator_ConfigDefaults(t *testing.T) {
	config := BPTValidatorConfig{}
	config.applyDefaults()

	assert.Equal(t, DefaultBPTValidationInterval, config.ValidationInterval)
	assert.Equal(t, DefaultBPTValidationMaxPasses, config.MaxPasses)
	assert.Equal(t, DefaultBPTValidationConvergenceThreshold, config.ConvergenceThreshold)
}

func TestBPTValidator_StateString(t *testing.T) {
	assert.Equal(t, "idle", ValidationStateIdle.String())
	assert.Equal(t, "running", ValidationStateRunning.String())
	assert.Equal(t, "converged", ValidationStateConverged.String())
	assert.Equal(t, "failed", ValidationStateFailed.String())
	assert.Equal(t, "unknown", ValidationState(99).String())
}

func TestBPTValidator_SuccessfulValidation(t *testing.T) {
	provider := newMockBPTValidationProvider()

	// Set matching root hashes
	var rootHash [32]byte
	_, err := rand.Read(rootHash[:])
	require.NoError(t, err)

	provider.SetExpectedRoot(&rootHash)
	provider.SetLocalRoot(rootHash)

	config := BPTValidatorConfig{
		ConvergenceThreshold: 1, // Quick convergence for test
	}
	validator := NewBPTValidator(config, provider, nil)

	// Track convergence callback
	var converged atomic.Bool
	validator.SetCallbacks(func() {
		converged.Store(true)
	}, nil)

	// Run a validation pass
	validator.runValidationPass()

	// Should have converged
	assert.True(t, converged.Load())
	assert.Equal(t, ValidationStateConverged, validator.State())
	assert.Equal(t, 1, validator.PassCount())
	assert.Equal(t, 0, validator.InvalidNodesCount())
}

func TestBPTValidator_FailedValidation_MaxPasses(t *testing.T) {
	provider := newMockBPTValidationProvider()

	// Set mismatched root hashes
	var expectedRoot, localRoot [32]byte
	_, _ = rand.Read(expectedRoot[:])
	_, _ = rand.Read(localRoot[:])

	provider.SetExpectedRoot(&expectedRoot)
	provider.SetLocalRoot(localRoot)

	config := BPTValidatorConfig{
		MaxPasses: 2, // Small for test
	}
	validator := NewBPTValidator(config, provider, nil)

	// Track failure callback
	var failed atomic.Bool
	var failReason string
	validator.SetCallbacks(nil, func(reason string) {
		failed.Store(true)
		failReason = reason
	})

	// Simulate invalid nodes that don't change
	validator.invalidNodesCount.Store(5)
	validator.lastInvalidCount.Store(5)

	// Run validation passes
	validator.runValidationPass()
	validator.runValidationPass()

	// Should have failed after max passes
	assert.True(t, failed.Load())
	assert.Contains(t, failReason, "max validation passes exceeded")
	assert.Equal(t, ValidationStateFailed, validator.State())
}

func TestBPTValidator_ProgressTracking(t *testing.T) {
	provider := newMockBPTValidationProvider()

	// Set mismatched root hashes
	var expectedRoot, localRoot [32]byte
	_, _ = rand.Read(expectedRoot[:])
	_, _ = rand.Read(localRoot[:])

	provider.SetExpectedRoot(&expectedRoot)
	provider.SetLocalRoot(localRoot)

	config := BPTValidatorConfig{
		MaxPasses: 10,
	}
	validator := NewBPTValidator(config, provider, nil)

	// Simulate progress (invalid count decreasing)
	validator.invalidNodesCount.Store(10)
	validator.runValidationPass()
	assert.Equal(t, int32(0), validator.unchangedPassCount.Load())

	// Simulate no progress
	validator.invalidNodesCount.Store(5)
	validator.lastInvalidCount.Store(5)
	validator.runValidationPass()
	assert.Equal(t, int32(1), validator.unchangedPassCount.Load())
}

func TestBPTValidator_Reset(t *testing.T) {
	provider := newMockBPTValidationProvider()
	config := BPTValidatorConfig{}
	validator := NewBPTValidator(config, provider, nil)

	// Set some state
	validator.state.Store(int32(ValidationStateRunning))
	validator.passCount.Store(5)
	validator.consecutiveSuccess.Store(2)
	validator.invalidNodesCount.Store(10)

	// Reset
	validator.Reset()

	// Verify reset
	assert.Equal(t, ValidationStateIdle, validator.State())
	assert.Equal(t, 0, validator.PassCount())
	assert.Equal(t, int32(0), validator.consecutiveSuccess.Load())
	assert.Equal(t, 0, validator.InvalidNodesCount())
}

func TestBPTValidator_Metrics(t *testing.T) {
	provider := newMockBPTValidationProvider()
	config := BPTValidatorConfig{}
	validator := NewBPTValidator(config, provider, nil)

	// Initially all zeros
	total, successful, failed := validator.Metrics()
	assert.Equal(t, uint64(0), total)
	assert.Equal(t, uint64(0), successful)
	assert.Equal(t, uint64(0), failed)

	// Increment via atomic operations
	validator.totalPasses.Add(10)
	validator.successfulPasses.Add(7)
	validator.failedPasses.Add(3)

	total, successful, failed = validator.Metrics()
	assert.Equal(t, uint64(10), total)
	assert.Equal(t, uint64(7), successful)
	assert.Equal(t, uint64(3), failed)
}

func TestBPTValidator_RecalculateHashesCalled(t *testing.T) {
	provider := newMockBPTValidationProvider()

	var rootHash [32]byte
	_, _ = rand.Read(rootHash[:])
	provider.SetExpectedRoot(&rootHash)
	provider.SetLocalRoot(rootHash)

	config := BPTValidatorConfig{
		ConvergenceThreshold: 1,
	}
	validator := NewBPTValidator(config, provider, nil)

	// Run a validation pass
	validator.runValidationPass()

	// RecalculateHashes should have been called
	assert.Equal(t, int32(1), provider.recalculateCalled.Load())
}

func TestBPTValidator_NoExpectedRoot(t *testing.T) {
	provider := newMockBPTValidationProvider()
	// No expected root set

	config := BPTValidatorConfig{}
	validator := NewBPTValidator(config, provider, nil)

	// Run a validation pass
	validator.runValidationPass()

	// Should have incremented total passes but not changed state
	total, _, _ := validator.Metrics()
	assert.Equal(t, uint64(1), total)
	assert.Equal(t, 1, validator.PassCount())
}

func TestBPTValidator_ConsecutiveSuccessThreshold(t *testing.T) {
	provider := newMockBPTValidationProvider()

	var rootHash [32]byte
	_, _ = rand.Read(rootHash[:])
	provider.SetExpectedRoot(&rootHash)
	provider.SetLocalRoot(rootHash)

	config := BPTValidatorConfig{
		ConvergenceThreshold: 3, // Need 3 consecutive successes
	}
	validator := NewBPTValidator(config, provider, nil)

	var converged atomic.Bool
	validator.SetCallbacks(func() {
		converged.Store(true)
	}, nil)

	// First two passes should not trigger convergence
	validator.runValidationPass()
	assert.False(t, converged.Load())
	assert.Equal(t, int32(1), validator.consecutiveSuccess.Load())

	validator.runValidationPass()
	assert.False(t, converged.Load())
	assert.Equal(t, int32(2), validator.consecutiveSuccess.Load())

	// Third pass should trigger convergence
	validator.runValidationPass()
	assert.True(t, converged.Load())
	assert.Equal(t, int32(3), validator.consecutiveSuccess.Load())
	assert.Equal(t, ValidationStateConverged, validator.State())
}

func TestBPTValidator_LastValidationError(t *testing.T) {
	provider := newMockBPTValidationProvider()
	config := BPTValidatorConfig{}
	validator := NewBPTValidator(config, provider, nil)

	// Initially empty
	assert.Equal(t, "", validator.LastValidationError())

	// Set an error
	errStr := "test error"
	validator.lastValidationErr.Store(&errStr)

	assert.Equal(t, "test error", validator.LastValidationError())
}

func TestBPTValidator_IntegrationWithWalker(t *testing.T) {
	hashProvider := newMockBPTHashProvider()
	validationProvider := newMockBPTValidationProvider()

	// Set up mismatched state
	var expectedRoot, localRoot [32]byte
	_, _ = rand.Read(expectedRoot[:])
	_, _ = rand.Read(localRoot[:])

	hashProvider.SetExpectedRoot(&expectedRoot)
	hashProvider.SetLocalRoot(localRoot)
	validationProvider.SetExpectedRoot(&expectedRoot)
	validationProvider.SetLocalRoot(localRoot)

	// Create walker
	walkerConfig := BPTWalkerConfig{
		WalkInterval: 10 * time.Millisecond,
	}
	walker := NewBPTWalker(walkerConfig, hashProvider, nil)

	// Mark walker as diverged
	walker.diverged.Store(true)

	// Create validator
	validatorConfig := BPTValidatorConfig{
		ValidationInterval: 10 * time.Millisecond,
		MaxPasses:          5,
	}
	validator := NewBPTValidator(validatorConfig, validationProvider, walker)

	// Run a validation pass
	validator.runValidationPass()

	// Should have triggered a walk
	assert.Equal(t, 1, validator.PassCount())
}
