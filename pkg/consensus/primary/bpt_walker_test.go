// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBPTHashProvider is a mock implementation of BPTHashProvider for testing.
type mockBPTHashProvider struct {
	mu            sync.RWMutex
	expectedRoot  *[32]byte
	localRoot     [32]byte
	localRootErr  error
	branchHashes  map[[32]byte][32]byte
	leafValues    map[[32]byte][]byte
	iterateKeys   [][32]byte
	iterateValues [][]byte
}

func newMockBPTHashProvider() *mockBPTHashProvider {
	return &mockBPTHashProvider{
		branchHashes: make(map[[32]byte][32]byte),
		leafValues:   make(map[[32]byte][]byte),
	}
}

func (p *mockBPTHashProvider) GetExpectedRootHash() *[32]byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.expectedRoot
}

func (p *mockBPTHashProvider) GetLocalRootHash() ([32]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.localRoot, p.localRootErr
}

func (p *mockBPTHashProvider) GetBranchHash(key [32]byte) ([32]byte, bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	hash, ok := p.branchHashes[key]
	return hash, ok, nil
}

func (p *mockBPTHashProvider) GetLeafValue(keyHash [32]byte) ([]byte, bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	val, ok := p.leafValues[keyHash]
	return val, ok, nil
}

func (p *mockBPTHashProvider) IterateKeys(fn func(keyHash [32]byte, value []byte) error) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for i, k := range p.iterateKeys {
		if err := fn(k, p.iterateValues[i]); err != nil {
			return err
		}
	}
	return nil
}

func (p *mockBPTHashProvider) SetExpectedRoot(root *[32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expectedRoot = root
}

func (p *mockBPTHashProvider) SetLocalRoot(root [32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.localRoot = root
}

func TestBPTWalker_NoExpectedHash(t *testing.T) {
	provider := newMockBPTHashProvider()
	// No expected root set

	config := BPTWalkerConfig{
		WalkInterval: 10 * time.Millisecond,
	}
	walker := NewBPTWalker(config, provider, nil)

	// Walk should do nothing when no expected hash is available
	walker.walk()

	assert.False(t, walker.IsDiverged())
	assert.Equal(t, uint64(0), walker.walkCycles.Load())
}

func TestBPTWalker_StateConsistent(t *testing.T) {
	provider := newMockBPTHashProvider()

	// Set matching root hashes
	var rootHash [32]byte
	_, err := rand.Read(rootHash[:])
	require.NoError(t, err)

	provider.SetExpectedRoot(&rootHash)
	provider.SetLocalRoot(rootHash)

	config := BPTWalkerConfig{
		WalkInterval: 10 * time.Millisecond,
	}
	walker := NewBPTWalker(config, provider, nil)

	// Walk should detect consistent state
	walker.walk()

	assert.False(t, walker.IsDiverged())
	assert.Equal(t, uint64(1), walker.walkCycles.Load())
}

func TestBPTWalker_StateDivergence(t *testing.T) {
	provider := newMockBPTHashProvider()

	// Set mismatched root hashes
	var expectedRoot, localRoot [32]byte
	_, _ = rand.Read(expectedRoot[:])
	_, _ = rand.Read(localRoot[:])

	provider.SetExpectedRoot(&expectedRoot)
	provider.SetLocalRoot(localRoot)

	config := BPTWalkerConfig{
		WalkInterval: 10 * time.Millisecond,
	}
	walker := NewBPTWalker(config, provider, nil)

	// Walk should detect divergence
	walker.walk()

	assert.True(t, walker.IsDiverged())
	assert.Equal(t, "root hash mismatch", walker.DivergenceReason())
	assert.Equal(t, uint64(1), walker.walkCycles.Load())
}

func TestBPTWalker_StateConvergence(t *testing.T) {
	provider := newMockBPTHashProvider()

	// Start with mismatched hashes
	var expectedRoot, localRoot [32]byte
	_, _ = rand.Read(expectedRoot[:])
	_, _ = rand.Read(localRoot[:])

	provider.SetExpectedRoot(&expectedRoot)
	provider.SetLocalRoot(localRoot)

	config := BPTWalkerConfig{
		WalkInterval: 10 * time.Millisecond,
	}
	walker := NewBPTWalker(config, provider, nil)

	// Walk should detect divergence
	walker.walk()
	assert.True(t, walker.IsDiverged())

	// Now make them match
	provider.SetLocalRoot(expectedRoot)

	// Walk should detect convergence
	walker.walk()
	assert.False(t, walker.IsDiverged())
	assert.Equal(t, "", walker.DivergenceReason())
}

func TestBPTWalker_OnEntryReceived(t *testing.T) {
	provider := newMockBPTHashProvider()
	config := BPTWalkerConfig{}
	walker := NewBPTWalker(config, provider, nil)

	// Add some missing keys
	var key1, key2, key3 [32]byte
	_, _ = rand.Read(key1[:])
	_, _ = rand.Read(key2[:])
	_, _ = rand.Read(key3[:])

	walker.missingKeysMu.Lock()
	walker.missingKeys = [][32]byte{key1, key2, key3}
	walker.missingKeysMu.Unlock()

	assert.Equal(t, 3, walker.PendingGapCount())

	// Receive one entry
	walker.OnEntryReceived(key2)

	assert.Equal(t, 2, walker.PendingGapCount())

	// Verify key2 was removed
	walker.missingKeysMu.Lock()
	found := false
	for _, k := range walker.missingKeys {
		if k == key2 {
			found = true
			break
		}
	}
	walker.missingKeysMu.Unlock()
	assert.False(t, found, "key2 should have been removed")
}

func TestBPTWalker_Metrics(t *testing.T) {
	provider := newMockBPTHashProvider()
	config := BPTWalkerConfig{}
	walker := NewBPTWalker(config, provider, nil)

	// Initially all zeros
	walkCycles, gapsFound, gapsResolved := walker.Metrics()
	assert.Equal(t, uint64(0), walkCycles)
	assert.Equal(t, uint64(0), gapsFound)
	assert.Equal(t, uint64(0), gapsResolved)

	// Increment via atomic operations
	walker.walkCycles.Add(5)
	walker.gapsFound.Add(10)
	walker.gapsResolved.Add(3)

	walkCycles, gapsFound, gapsResolved = walker.Metrics()
	assert.Equal(t, uint64(5), walkCycles)
	assert.Equal(t, uint64(10), gapsFound)
	assert.Equal(t, uint64(3), gapsResolved)
}

func TestBPTWalker_LastWalkTime(t *testing.T) {
	provider := newMockBPTHashProvider()
	config := BPTWalkerConfig{}
	walker := NewBPTWalker(config, provider, nil)

	// Initially zero time
	assert.True(t, walker.LastWalkTime().IsZero())

	// Set matching hashes to trigger a successful walk
	var rootHash [32]byte
	_, _ = rand.Read(rootHash[:])
	provider.SetExpectedRoot(&rootHash)
	provider.SetLocalRoot(rootHash)

	// Walk
	before := time.Now()
	walker.walk()
	after := time.Now()

	// Check walk time was updated
	walkTime := walker.LastWalkTime()
	assert.True(t, walkTime.After(before) || walkTime.Equal(before))
	assert.True(t, walkTime.Before(after) || walkTime.Equal(after))
}

func TestBPTWalker_PendingGapCount(t *testing.T) {
	provider := newMockBPTHashProvider()
	config := BPTWalkerConfig{}
	walker := NewBPTWalker(config, provider, nil)

	assert.Equal(t, 0, walker.PendingGapCount())

	// Add some missing keys
	walker.missingKeysMu.Lock()
	for i := 0; i < 5; i++ {
		var key [32]byte
		_, _ = rand.Read(key[:])
		walker.missingKeys = append(walker.missingKeys, key)
	}
	walker.missingKeysMu.Unlock()

	assert.Equal(t, 5, walker.PendingGapCount())
}

func TestBPTWalker_MaxPendingRequests(t *testing.T) {
	provider := newMockBPTHashProvider()

	// Set mismatched hashes
	var expectedRoot, localRoot [32]byte
	_, _ = rand.Read(expectedRoot[:])
	_, _ = rand.Read(localRoot[:])
	provider.SetExpectedRoot(&expectedRoot)
	provider.SetLocalRoot(localRoot)

	config := BPTWalkerConfig{
		MaxPendingRequests: 3,
	}
	walker := NewBPTWalker(config, provider, nil)

	// Pre-populate some missing keys
	walker.missingKeysMu.Lock()
	for i := 0; i < 5; i++ {
		var key [32]byte
		_, _ = rand.Read(key[:])
		walker.missingKeys = append(walker.missingKeys, key)
	}
	walker.missingKeysMu.Unlock()

	// Walk should not add more keys beyond the limit
	walker.walk()

	// Since we already have 5 (which is > max of 3), no new keys should be added
	// The current implementation still has 5 because we started with 5
	assert.Equal(t, 5, walker.PendingGapCount())
}

func TestBPTWalker_ConfigDefaults(t *testing.T) {
	config := BPTWalkerConfig{}
	config.applyDefaults()

	assert.Equal(t, DefaultBPTWalkInterval, config.WalkInterval)
	assert.Equal(t, DefaultBPTWalkBatchSize, config.BatchSize)
	assert.Equal(t, DefaultBPTWalkMaxPendingRequests, config.MaxPendingRequests)
}
