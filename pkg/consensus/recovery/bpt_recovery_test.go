// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package recovery

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockBPTRecoveryProvider is a mock implementation of BPTRecoveryProvider.
type mockBPTRecoveryProvider struct {
	mu            sync.RWMutex
	entries       map[[32]byte][]byte
	expectedRoot  *[32]byte
	localRoot     [32]byte
	chainHeads    map[[32]byte]struct{ count uint64; anchor [32]byte }
	expectedAnchors map[[32]byte][32]byte
}

func newMockBPTRecoveryProvider() *mockBPTRecoveryProvider {
	return &mockBPTRecoveryProvider{
		entries:         make(map[[32]byte][]byte),
		chainHeads:      make(map[[32]byte]struct{ count uint64; anchor [32]byte }),
		expectedAnchors: make(map[[32]byte][32]byte),
	}
}

// BPTStore interface
func (p *mockBPTRecoveryProvider) GetEntry(keyHash [32]byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.entries[keyHash], nil
}

func (p *mockBPTRecoveryProvider) PutEntry(keyHash [32]byte, value []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[keyHash] = value
	return nil
}

func (p *mockBPTRecoveryProvider) HasEntry(keyHash [32]byte) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.entries[keyHash]
	return ok, nil
}

// BPTHashProvider interface
func (p *mockBPTRecoveryProvider) GetExpectedRootHash() *[32]byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.expectedRoot
}

func (p *mockBPTRecoveryProvider) GetLocalRootHash() ([32]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.localRoot, nil
}

func (p *mockBPTRecoveryProvider) GetBranchHash(key [32]byte) ([32]byte, bool, error) {
	return [32]byte{}, false, nil
}

func (p *mockBPTRecoveryProvider) GetLeafValue(keyHash [32]byte) ([]byte, bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.entries[keyHash]
	return v, ok, nil
}

func (p *mockBPTRecoveryProvider) IterateKeys(fn func(keyHash [32]byte, value []byte) error) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for k, v := range p.entries {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

// BPTValidationProvider interface
func (p *mockBPTRecoveryProvider) RecalculateHashes() error {
	return nil
}

// ChainEntryProvider interface
func (p *mockBPTRecoveryProvider) GetChainHead(chainKey [32]byte) (count uint64, anchor [32]byte, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	head, ok := p.chainHeads[chainKey]
	if !ok {
		return 0, [32]byte{}, nil
	}
	return head.count, head.anchor, nil
}

func (p *mockBPTRecoveryProvider) GetExpectedChainAnchor(chainKey [32]byte) ([32]byte, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	anchor, ok := p.expectedAnchors[chainKey]
	return anchor, ok
}

func (p *mockBPTRecoveryProvider) IterateChainKeys(fn func(chainKey [32]byte, expectedAnchor [32]byte) error) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for k, anchor := range p.expectedAnchors {
		if err := fn(k, anchor); err != nil {
			return err
		}
	}
	return nil
}

// Helper methods for testing
func (p *mockBPTRecoveryProvider) SetExpectedRoot(root *[32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expectedRoot = root
}

func (p *mockBPTRecoveryProvider) SetLocalRoot(root [32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.localRoot = root
}

// mockChainEntryRequester is a mock implementation for testing.
type mockChainEntryRequester struct {
	mu      sync.RWMutex
	entries map[[32]byte][][]byte
}

func newMockChainEntryRequester() *mockChainEntryRequester {
	return &mockChainEntryRequester{
		entries: make(map[[32]byte][][]byte),
	}
}

func (r *mockChainEntryRequester) RequestChainEntries(ctx context.Context, chainKey [32]byte, startIndex, count uint64) ([][]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return nil, nil
}

func (r *mockChainEntryRequester) InsertChainEntry(chainKey [32]byte, index uint64, entry []byte) error {
	return nil
}

func TestRecoveryState_String(t *testing.T) {
	assert.Equal(t, "idle", RecoveryStateIdle.String())
	assert.Equal(t, "detecting", RecoveryStateDetecting.String())
	assert.Equal(t, "syncing", RecoveryStateSyncing.String())
	assert.Equal(t, "validating", RecoveryStateValidating.String())
	assert.Equal(t, "filling_chains", RecoveryStateFillingChains.String())
	assert.Equal(t, "complete", RecoveryStateComplete.String())
	assert.Equal(t, "failed", RecoveryStateFailed.String())
	assert.Equal(t, "unknown", RecoveryState(99).String())
}

func TestBPTRecoveryCoordinator_Create(t *testing.T) {
	provider := newMockBPTRecoveryProvider()
	requester := newMockChainEntryRequester()
	config := BPTRecoveryConfig{}

	coord := NewBPTRecoveryCoordinator(config, provider, nil, requester)
	assert.NotNil(t, coord)
	assert.Equal(t, RecoveryStateIdle, coord.State())
}

func TestBPTRecoveryCoordinator_Status(t *testing.T) {
	provider := newMockBPTRecoveryProvider()
	requester := newMockChainEntryRequester()
	config := BPTRecoveryConfig{}

	coord := NewBPTRecoveryCoordinator(config, provider, nil, requester)

	status := coord.Status()
	assert.Equal(t, RecoveryStateIdle, status.State)
	assert.Equal(t, 0, status.FailureCount)
}

func TestBPTRecoveryCoordinator_StateTransitions(t *testing.T) {
	provider := newMockBPTRecoveryProvider()
	requester := newMockChainEntryRequester()

	// Set up matching roots (no divergence)
	var rootHash [32]byte
	_, _ = rand.Read(rootHash[:])
	provider.SetExpectedRoot(&rootHash)
	provider.SetLocalRoot(rootHash)

	config := BPTRecoveryConfig{}
	coord := NewBPTRecoveryCoordinator(config, provider, nil, requester)

	// Initially idle
	assert.Equal(t, RecoveryStateIdle, coord.State())

	// Manually set state to test transitions
	coord.state.Store(int32(RecoveryStateSyncing))
	assert.Equal(t, RecoveryStateSyncing, coord.State())

	coord.state.Store(int32(RecoveryStateValidating))
	assert.Equal(t, RecoveryStateValidating, coord.State())

	coord.state.Store(int32(RecoveryStateFillingChains))
	assert.Equal(t, RecoveryStateFillingChains, coord.State())

	coord.state.Store(int32(RecoveryStateComplete))
	assert.Equal(t, RecoveryStateComplete, coord.State())
}

func TestBPTRecoveryCoordinator_Callbacks(t *testing.T) {
	provider := newMockBPTRecoveryProvider()
	requester := newMockChainEntryRequester()
	config := BPTRecoveryConfig{}

	coord := NewBPTRecoveryCoordinator(config, provider, nil, requester)

	var completeCalled, failedCalled bool
	var failReason string

	coord.SetCallbacks(
		func() { completeCalled = true },
		func(reason string) {
			failedCalled = true
			failReason = reason
		},
	)

	// Test complete callback
	coord.onRecoveryComplete()
	assert.True(t, completeCalled)
	assert.Equal(t, RecoveryStateComplete, coord.State())

	// Reset and test failed callback
	coord.state.Store(int32(RecoveryStateIdle))
	coord.onRecoveryTimeout()
	assert.True(t, failedCalled)
	assert.Equal(t, "recovery timeout", failReason)
	assert.Equal(t, RecoveryStateFailed, coord.State())
}

func TestBPTRecoveryCoordinator_ValidationFailureRetry(t *testing.T) {
	provider := newMockBPTRecoveryProvider()
	requester := newMockChainEntryRequester()
	config := BPTRecoveryConfig{}

	coord := NewBPTRecoveryCoordinator(config, provider, nil, requester)
	coord.state.Store(int32(RecoveryStateValidating))

	// First failure - should retry
	coord.onValidationFailed("test failure 1")
	assert.Equal(t, RecoveryStateSyncing, coord.State())
	assert.Equal(t, int32(1), coord.failureCount.Load())

	// Second failure - should retry
	coord.onValidationFailed("test failure 2")
	assert.Equal(t, RecoveryStateSyncing, coord.State())
	assert.Equal(t, int32(2), coord.failureCount.Load())

	// Third failure - should fail completely
	var failedReason string
	coord.SetCallbacks(nil, func(reason string) {
		failedReason = reason
	})
	coord.onValidationFailed("test failure 3")
	assert.Equal(t, RecoveryStateFailed, coord.State())
	assert.Contains(t, failedReason, "validation failed after multiple attempts")
}

func TestBPTRecoveryCoordinator_Metrics(t *testing.T) {
	provider := newMockBPTRecoveryProvider()
	requester := newMockChainEntryRequester()
	config := BPTRecoveryConfig{}

	coord := NewBPTRecoveryCoordinator(config, provider, nil, requester)

	syncerMetrics, walkerMetrics, validatorMetrics, chainFillerMetrics := coord.Metrics()

	// All metrics should be zero initially
	assert.Equal(t, [3]uint64{0, 0, 0}, syncerMetrics)
	assert.Equal(t, [3]uint64{0, 0, 0}, walkerMetrics)
	assert.Equal(t, [3]uint64{0, 0, 0}, validatorMetrics)
	assert.Equal(t, [3]uint64{0, 0, 0}, chainFillerMetrics)
}
