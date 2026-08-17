// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChainEntryProvider is a mock implementation for testing.
type mockChainEntryProvider struct {
	mu         sync.RWMutex
	chainHeads map[[32]byte]struct {
		count  uint64
		anchor [32]byte
	}
	expectedAnchors map[[32]byte][32]byte
	chainKeys       [][32]byte
}

func newMockChainEntryProvider() *mockChainEntryProvider {
	return &mockChainEntryProvider{
		chainHeads: make(map[[32]byte]struct {
			count  uint64
			anchor [32]byte
		}),
		expectedAnchors: make(map[[32]byte][32]byte),
	}
}

func (p *mockChainEntryProvider) GetChainHead(chainKey [32]byte) (count uint64, anchor [32]byte, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	head, ok := p.chainHeads[chainKey]
	if !ok {
		return 0, [32]byte{}, nil
	}
	return head.count, head.anchor, nil
}

func (p *mockChainEntryProvider) GetExpectedChainAnchor(chainKey [32]byte) ([32]byte, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	anchor, ok := p.expectedAnchors[chainKey]
	return anchor, ok
}

func (p *mockChainEntryProvider) IterateChainKeys(fn func(chainKey [32]byte, expectedAnchor [32]byte) error) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, key := range p.chainKeys {
		if anchor, ok := p.expectedAnchors[key]; ok {
			if err := fn(key, anchor); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *mockChainEntryProvider) SetChainHead(chainKey [32]byte, count uint64, anchor [32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chainHeads[chainKey] = struct {
		count  uint64
		anchor [32]byte
	}{count, anchor}
}

func (p *mockChainEntryProvider) SetExpectedAnchor(chainKey [32]byte, anchor [32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expectedAnchors[chainKey] = anchor
	// Add to chain keys if not present
	for _, k := range p.chainKeys {
		if k == chainKey {
			return
		}
	}
	p.chainKeys = append(p.chainKeys, chainKey)
}

// mockChainEntryRequester is a mock implementation for testing.
type mockChainEntryRequester struct {
	mu             sync.RWMutex
	entries        map[[32]byte][][]byte
	insertedCount  int
	requestedCount int
}

func newMockChainEntryRequester() *mockChainEntryRequester {
	return &mockChainEntryRequester{
		entries: make(map[[32]byte][][]byte),
	}
}

func (r *mockChainEntryRequester) RequestChainEntries(ctx context.Context, chainKey [32]byte, startIndex, count uint64) ([][]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requestedCount++

	entries, ok := r.entries[chainKey]
	if !ok {
		return nil, nil
	}

	// Return entries from startIndex
	if startIndex >= uint64(len(entries)) {
		return nil, nil
	}

	end := startIndex + count
	if end > uint64(len(entries)) {
		end = uint64(len(entries))
	}

	return entries[startIndex:end], nil
}

func (r *mockChainEntryRequester) InsertChainEntry(chainKey [32]byte, index uint64, entry []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.insertedCount++
	return nil
}

func (r *mockChainEntryRequester) SetEntries(chainKey [32]byte, entries [][]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[chainKey] = entries
}

func TestChainGapFiller_ConfigDefaults(t *testing.T) {
	config := ChainGapFillerConfig{}
	config.applyDefaults()

	assert.Equal(t, DefaultChainGapCheckInterval, config.CheckInterval)
	assert.Equal(t, DefaultChainGapBatchSize, config.BatchSize)
	assert.Equal(t, DefaultChainGapMaxPending, config.MaxPending)
}

func TestChainGapFiller_AddGap(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{}
	filler := NewChainGapFiller(config, provider, requester)

	var chainKey [32]byte
	_, _ = rand.Read(chainKey[:])

	gap := ChainGap{
		ChainKey:   chainKey,
		StartIndex: 10,
		EndIndex:   20,
	}

	filler.AddGap(gap)
	assert.Equal(t, 1, filler.PendingGapCount())

	// Adding same gap again should not increase count
	filler.AddGap(gap)
	assert.Equal(t, 1, filler.PendingGapCount())

	// Adding different gap should increase count
	gap2 := ChainGap{
		ChainKey:   chainKey,
		StartIndex: 30,
		EndIndex:   40,
	}
	filler.AddGap(gap2)
	assert.Equal(t, 2, filler.PendingGapCount())
}

func TestChainGapFiller_MaxPending(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{
		MaxPending: 3,
	}
	filler := NewChainGapFiller(config, provider, requester)

	// Add gaps up to max
	for i := 0; i < 5; i++ {
		var chainKey [32]byte
		_, _ = rand.Read(chainKey[:])
		gap := ChainGap{
			ChainKey:   chainKey,
			StartIndex: uint64(i * 10),
			EndIndex:   uint64((i + 1) * 10),
		}
		filler.AddGap(gap)
	}

	// Should be capped at max
	assert.Equal(t, 3, filler.PendingGapCount())
}

func TestChainGapFiller_DetectGaps(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{}
	filler := NewChainGapFiller(config, provider, requester)

	var chainKey [32]byte
	_, _ = rand.Read(chainKey[:])

	var localAnchor, expectedAnchor [32]byte
	_, _ = rand.Read(localAnchor[:])
	_, _ = rand.Read(expectedAnchor[:])

	// Set up a gap: local anchor != expected anchor
	provider.SetChainHead(chainKey, 100, localAnchor)
	provider.SetExpectedAnchor(chainKey, expectedAnchor)

	// Detect gaps
	filler.detectGaps()

	// Should have detected one gap
	assert.Equal(t, 1, filler.PendingGapCount())
}

func TestChainGapFiller_NoGapWhenAnchorsMatch(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{}
	filler := NewChainGapFiller(config, provider, requester)

	var chainKey [32]byte
	_, _ = rand.Read(chainKey[:])

	var anchor [32]byte
	_, _ = rand.Read(anchor[:])

	// Set up matching anchors
	provider.SetChainHead(chainKey, 100, anchor)
	provider.SetExpectedAnchor(chainKey, anchor)

	// Detect gaps
	filler.detectGaps()

	// Should not detect any gaps
	assert.Equal(t, 0, filler.PendingGapCount())
}

func TestChainGapFiller_FillGap(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{
		BatchSize: 10,
	}
	filler := NewChainGapFiller(config, provider, requester)

	var chainKey [32]byte
	_, _ = rand.Read(chainKey[:])

	// Set up entries to return
	entries := make([][]byte, 5)
	for i := range entries {
		entries[i] = []byte{byte(i)}
	}
	requester.SetEntries(chainKey, entries)

	var expectedAnchor [32]byte
	_, _ = rand.Read(expectedAnchor[:])

	// Set up provider to return matching anchor after fill
	provider.SetExpectedAnchor(chainKey, expectedAnchor)
	provider.SetChainHead(chainKey, 5, expectedAnchor) // After fill, it matches

	// Add a gap
	gap := ChainGap{
		ChainKey:       chainKey,
		StartIndex:     0,
		EndIndex:       10,
		ExpectedAnchor: expectedAnchor,
	}
	filler.AddGap(gap)
	assert.Equal(t, 1, filler.PendingGapCount())

	// Fill gaps
	filler.fillGaps()

	// Gap should be removed
	assert.Equal(t, 0, filler.PendingGapCount())

	// Entries should have been inserted
	assert.Equal(t, 5, requester.insertedCount)
}

func TestChainGapFiller_Metrics(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{}
	filler := NewChainGapFiller(config, provider, requester)

	// Initially all zeros
	gapsDetected, gapsFilled, entriesRecv := filler.Metrics()
	assert.Equal(t, uint64(0), gapsDetected)
	assert.Equal(t, uint64(0), gapsFilled)
	assert.Equal(t, uint64(0), entriesRecv)

	// Increment via atomic operations
	filler.gapsDetected.Add(5)
	filler.gapsFilled.Add(3)
	filler.entriesRecv.Add(50)

	gapsDetected, gapsFilled, entriesRecv = filler.Metrics()
	assert.Equal(t, uint64(5), gapsDetected)
	assert.Equal(t, uint64(3), gapsFilled)
	assert.Equal(t, uint64(50), entriesRecv)
}

func TestChainGapFiller_StartStop(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{
		CheckInterval: 10 * time.Millisecond,
	}
	filler := NewChainGapFiller(config, provider, requester)

	ctx := context.Background()
	err := filler.Start(ctx)
	require.NoError(t, err)

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)

	// Check cycles should have run
	assert.Greater(t, filler.checkCycles.Load(), uint64(0))

	filler.Stop()
}

func TestChainGapFiller_TriggerCheck(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{
		CheckInterval: 1 * time.Hour, // Long interval so we know trigger works
	}
	filler := NewChainGapFiller(config, provider, requester)

	ctx := context.Background()
	err := filler.Start(ctx)
	require.NoError(t, err)
	defer filler.Stop()

	// Trigger check
	filler.TriggerCheck()

	// Wait a bit for async execution
	time.Sleep(50 * time.Millisecond)

	// Check should have run
	assert.Greater(t, filler.checkCycles.Load(), uint64(0))
}

func TestChainGapFiller_RemoveGap(t *testing.T) {
	provider := newMockChainEntryProvider()
	requester := newMockChainEntryRequester()
	config := ChainGapFillerConfig{}
	filler := NewChainGapFiller(config, provider, requester)

	var chainKey1, chainKey2 [32]byte
	_, _ = rand.Read(chainKey1[:])
	_, _ = rand.Read(chainKey2[:])

	gap1 := ChainGap{ChainKey: chainKey1, StartIndex: 10}
	gap2 := ChainGap{ChainKey: chainKey2, StartIndex: 20}

	filler.AddGap(gap1)
	filler.AddGap(gap2)
	assert.Equal(t, 2, filler.PendingGapCount())

	filler.removeGap(gap1)
	assert.Equal(t, 1, filler.PendingGapCount())

	// Remaining gap should be gap2
	filler.gapsMu.Lock()
	assert.Equal(t, chainKey2, filler.gaps[0].ChainKey)
	filler.gapsMu.Unlock()
}
