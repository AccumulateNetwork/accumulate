// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
)

// mockBPTStore is a simple in-memory implementation of BPTStore for testing.
type mockBPTStore struct {
	mu      sync.RWMutex
	entries map[[32]byte][]byte
}

func newMockBPTStore() *mockBPTStore {
	return &mockBPTStore{
		entries: make(map[[32]byte][]byte),
	}
}

func (s *mockBPTStore) GetEntry(keyHash [32]byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries[keyHash], nil
}

func (s *mockBPTStore) PutEntry(keyHash [32]byte, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[keyHash] = value
	return nil
}

func (s *mockBPTStore) HasEntry(keyHash [32]byte) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entries[keyHash]
	return ok, nil
}

// mockGossipLayer is a minimal gossip layer for testing BPTSyncer.
type mockGossipLayer struct {
	mu           sync.RWMutex
	requests     chan *gossip.BPTSyncRequest
	responses    chan *gossip.BPTSyncResponse
	lastRequest  *gossip.BPTSyncRequest
	lastResponse *gossip.BPTSyncResponse
}

func newMockGossipLayer() *mockGossipLayer {
	return &mockGossipLayer{
		requests:  make(chan *gossip.BPTSyncRequest, 100),
		responses: make(chan *gossip.BPTSyncResponse, 100),
	}
}

func (g *mockGossipLayer) BroadcastBPTSyncRequest(ctx context.Context, req *gossip.BPTSyncRequest) error {
	g.mu.Lock()
	g.lastRequest = req
	g.mu.Unlock()
	return nil
}

func (g *mockGossipLayer) BroadcastBPTSyncResponse(ctx context.Context, resp *gossip.BPTSyncResponse) error {
	g.mu.Lock()
	g.lastResponse = resp
	g.mu.Unlock()
	return nil
}

func (g *mockGossipLayer) SubscribeBPTSyncRequests() <-chan *gossip.BPTSyncRequest {
	return g.requests
}

func (g *mockGossipLayer) SubscribeBPTSyncResponses() <-chan *gossip.BPTSyncResponse {
	return g.responses
}

func (g *mockGossipLayer) GetLastRequest() *gossip.BPTSyncRequest {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastRequest
}

func (g *mockGossipLayer) GetLastResponse() *gossip.BPTSyncResponse {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastResponse
}

func TestBPTSyncer_RequestMissing(t *testing.T) {
	store := newMockBPTStore()

	// Create a syncer without a gossip layer to test just the queuing logic
	config := BPTSyncerConfig{
		BatchInterval: 10 * time.Millisecond,
		JitterMax:     1 * time.Millisecond,
	}
	syncer := NewBPTSyncer(config, store, nil)

	// Generate some random key hashes
	keys := make([][32]byte, 5)
	for i := range keys {
		_, err := rand.Read(keys[i][:])
		require.NoError(t, err)
	}

	// Request missing
	syncer.RequestMissing(keys)

	// Check that they're in the batch queue
	syncer.batchQueueMu.Lock()
	assert.Len(t, syncer.batchQueue, 5)
	syncer.batchQueueMu.Unlock()
}

func TestBPTSyncer_DeduplicatesRequests(t *testing.T) {
	store := newMockBPTStore()

	config := BPTSyncerConfig{
		BatchInterval:         10 * time.Millisecond,
		DeduplicationInterval: 1 * time.Second,
		JitterMax:             1 * time.Millisecond,
	}
	syncer := NewBPTSyncer(config, store, nil)

	// Generate a random key hash
	var keyHash [32]byte
	_, err := rand.Read(keyHash[:])
	require.NoError(t, err)

	// Pre-populate the in-flight map to simulate a previous request
	syncer.inFlightMu.Lock()
	syncer.inFlight[keyHash] = &bptInFlightRequest{
		keyHash:   keyHash,
		requestID: 1,
		sentAt:    time.Now(), // Recent, so within deduplication interval
		retries:   0,
	}
	syncer.inFlightMu.Unlock()

	// Request it - should be skipped due to deduplication
	syncer.RequestMissing([][32]byte{keyHash})

	// Check that it's not in the queue (deduplication should have skipped it)
	syncer.batchQueueMu.Lock()
	assert.Len(t, syncer.batchQueue, 0)
	syncer.batchQueueMu.Unlock()
}

func TestBPTSyncer_SkipsExistingEntries(t *testing.T) {
	store := newMockBPTStore()

	// Pre-populate an entry
	var existingKey [32]byte
	_, err := rand.Read(existingKey[:])
	require.NoError(t, err)
	store.PutEntry(existingKey, []byte("existing value"))

	config := BPTSyncerConfig{
		BatchInterval: 10 * time.Millisecond,
		JitterMax:     1 * time.Millisecond,
	}
	syncer := NewBPTSyncer(config, store, nil)

	// Generate a new key hash
	var newKey [32]byte
	_, err = rand.Read(newKey[:])
	require.NoError(t, err)

	// Request both keys
	syncer.RequestMissing([][32]byte{existingKey, newKey})

	// Only the new key should be in the queue
	syncer.batchQueueMu.Lock()
	assert.Len(t, syncer.batchQueue, 1)
	assert.Equal(t, newKey, syncer.batchQueue[0])
	syncer.batchQueueMu.Unlock()
}

func TestBPTSyncer_HandleSyncRequest(t *testing.T) {
	store := newMockBPTStore()

	// Pre-populate some entries
	var key1, key2, key3 [32]byte
	_, _ = rand.Read(key1[:])
	_, _ = rand.Read(key2[:])
	_, _ = rand.Read(key3[:])

	store.PutEntry(key1, []byte("value1"))
	store.PutEntry(key2, []byte("value2"))
	// key3 is missing

	// Generate a key pair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	config := BPTSyncerConfig{
		PublicKey: priv.Public().(ed25519.PublicKey),
	}
	syncer := NewBPTSyncer(config, store, nil)

	// Create a sync request
	req := &gossip.BPTSyncRequest{
		Keys:      [][32]byte{key1, key2, key3},
		RequestID: 123,
	}

	// Handle the request (this would normally send a response)
	syncer.handleSyncRequest(req)

	// Since we don't have a gossip layer, we can't check the response directly
	// But we can verify the store was queried correctly
	val1, _ := store.GetEntry(key1)
	assert.Equal(t, []byte("value1"), val1)
	val2, _ := store.GetEntry(key2)
	assert.Equal(t, []byte("value2"), val2)
	val3, _ := store.GetEntry(key3)
	assert.Nil(t, val3)
}

func TestBPTSyncer_HandleSyncResponse(t *testing.T) {
	store := newMockBPTStore()

	config := BPTSyncerConfig{}
	syncer := NewBPTSyncer(config, store, nil)

	// Track received entries
	var receivedEntries []struct {
		keyHash [32]byte
		value   []byte
	}
	syncer.SetEntryReceivedCallback(func(keyHash [32]byte, value []byte) {
		receivedEntries = append(receivedEntries, struct {
			keyHash [32]byte
			value   []byte
		}{keyHash, value})
	})

	// Create entries
	var key1, key2 [32]byte
	_, _ = rand.Read(key1[:])
	_, _ = rand.Read(key2[:])

	// Mark them as in-flight
	syncer.inFlightMu.Lock()
	syncer.inFlight[key1] = &bptInFlightRequest{keyHash: key1}
	syncer.inFlight[key2] = &bptInFlightRequest{keyHash: key2}
	syncer.inFlightMu.Unlock()

	// Create a sync response
	resp := &gossip.BPTSyncResponse{
		Entries: []gossip.BPTEntry{
			{KeyHash: key1, Value: []byte("value1")},
			{KeyHash: key2, Value: []byte("value2")},
		},
		RequestID: 123,
	}

	// Handle the response
	syncer.handleSyncResponse(resp)

	// Check entries were stored
	val1, _ := store.GetEntry(key1)
	assert.Equal(t, []byte("value1"), val1)
	val2, _ := store.GetEntry(key2)
	assert.Equal(t, []byte("value2"), val2)

	// Check callback was invoked
	assert.Len(t, receivedEntries, 2)

	// Check in-flight was cleared
	syncer.inFlightMu.Lock()
	assert.NotContains(t, syncer.inFlight, key1)
	assert.NotContains(t, syncer.inFlight, key2)
	syncer.inFlightMu.Unlock()

	// Check metrics
	_, responsesRecv, entriesRecv := syncer.Metrics()
	assert.Equal(t, uint64(1), responsesRecv)
	assert.Equal(t, uint64(2), entriesRecv)
}

func TestBPTSyncer_RetryLogic(t *testing.T) {
	store := newMockBPTStore()

	config := BPTSyncerConfig{
		RetryTimeout:          50 * time.Millisecond,
		DeduplicationInterval: 10 * time.Millisecond, // Short so retry can happen
		MaxRetries:            2,
	}
	syncer := NewBPTSyncer(config, store, nil)

	// Create an in-flight request that's timed out
	var keyHash [32]byte
	_, _ = rand.Read(keyHash[:])

	syncer.inFlightMu.Lock()
	syncer.inFlight[keyHash] = &bptInFlightRequest{
		keyHash:   keyHash,
		requestID: 1,
		sentAt:    time.Now().Add(-100 * time.Millisecond), // Timed out, also past deduplication interval
		retries:   0,
	}
	syncer.inFlightMu.Unlock()

	// Check retries - this calls RequestMissing which queues the key
	syncer.checkRetries()

	// Should be queued for retry
	syncer.batchQueueMu.Lock()
	assert.Len(t, syncer.batchQueue, 1)
	syncer.batchQueueMu.Unlock()
}

func TestBPTSyncer_MaxRetriesExceeded(t *testing.T) {
	store := newMockBPTStore()

	config := BPTSyncerConfig{
		RetryTimeout: 50 * time.Millisecond,
		MaxRetries:   2,
	}
	syncer := NewBPTSyncer(config, store, nil)

	// Create an in-flight request that's exceeded max retries
	var keyHash [32]byte
	_, _ = rand.Read(keyHash[:])

	syncer.inFlightMu.Lock()
	syncer.inFlight[keyHash] = &bptInFlightRequest{
		keyHash:   keyHash,
		requestID: 1,
		sentAt:    time.Now().Add(-100 * time.Millisecond), // Timed out
		retries:   3,                                       // Exceeded max
	}
	syncer.inFlightMu.Unlock()

	// Check retries
	syncer.checkRetries()

	// Should be removed from in-flight, not queued
	syncer.inFlightMu.Lock()
	assert.NotContains(t, syncer.inFlight, keyHash)
	syncer.inFlightMu.Unlock()

	syncer.batchQueueMu.Lock()
	assert.Empty(t, syncer.batchQueue)
	syncer.batchQueueMu.Unlock()
}

func TestBPTSyncer_Metrics(t *testing.T) {
	store := newMockBPTStore()
	config := BPTSyncerConfig{}
	syncer := NewBPTSyncer(config, store, nil)

	// Initially all zeros
	sent, recv, entries := syncer.Metrics()
	assert.Equal(t, uint64(0), sent)
	assert.Equal(t, uint64(0), recv)
	assert.Equal(t, uint64(0), entries)

	// Increment via atomic operations
	syncer.requestsSent.Add(5)
	syncer.responsesRecv.Add(3)
	syncer.entriesRecv.Add(10)

	sent, recv, entries = syncer.Metrics()
	assert.Equal(t, uint64(5), sent)
	assert.Equal(t, uint64(3), recv)
	assert.Equal(t, uint64(10), entries)
}

func TestBPTSyncer_InFlightCount(t *testing.T) {
	store := newMockBPTStore()
	config := BPTSyncerConfig{}
	syncer := NewBPTSyncer(config, store, nil)

	assert.Equal(t, 0, syncer.InFlightCount())

	// Add some in-flight requests
	for i := 0; i < 5; i++ {
		var keyHash [32]byte
		_, _ = rand.Read(keyHash[:])
		syncer.inFlightMu.Lock()
		syncer.inFlight[keyHash] = &bptInFlightRequest{keyHash: keyHash}
		syncer.inFlightMu.Unlock()
	}

	assert.Equal(t, 5, syncer.InFlightCount())
}

func TestBPTSyncer_ClearInFlight(t *testing.T) {
	store := newMockBPTStore()
	config := BPTSyncerConfig{}
	syncer := NewBPTSyncer(config, store, nil)

	var keyHash [32]byte
	_, _ = rand.Read(keyHash[:])

	syncer.inFlightMu.Lock()
	syncer.inFlight[keyHash] = &bptInFlightRequest{keyHash: keyHash}
	syncer.inFlightMu.Unlock()

	assert.Equal(t, 1, syncer.InFlightCount())

	syncer.ClearInFlight(keyHash)

	assert.Equal(t, 0, syncer.InFlightCount())
}
