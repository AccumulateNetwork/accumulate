// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
)

// Default configuration values for BPT syncing.
const (
	// DefaultBPTSyncBatchInterval is the time to collect missing keys before sending a request.
	DefaultBPTSyncBatchInterval = 100 * time.Millisecond

	// DefaultBPTSyncDeduplicationInterval is the minimum time between requesting the same key.
	DefaultBPTSyncDeduplicationInterval = 10 * time.Second

	// DefaultBPTSyncRetryTimeout is the time to wait before retrying a request.
	DefaultBPTSyncRetryTimeout = 30 * time.Second

	// DefaultBPTSyncMaxRetries is the maximum number of retry attempts.
	DefaultBPTSyncMaxRetries = 5

	// DefaultBPTSyncJitterMax is the maximum random jitter added to requests.
	DefaultBPTSyncJitterMax = 200 * time.Millisecond
)

// BPTStore provides access to BPT entries for sync operations.
// Implementations should be thread-safe.
type BPTStore interface {
	// GetEntry retrieves the value for a BPT key hash.
	// Returns nil if the entry does not exist.
	GetEntry(keyHash [32]byte) ([]byte, error)

	// PutEntry stores a BPT entry.
	PutEntry(keyHash [32]byte, value []byte) error

	// HasEntry checks if an entry exists for the given key hash.
	HasEntry(keyHash [32]byte) (bool, error)
}

// BPTSyncerConfig holds configuration for the BPTSyncer.
type BPTSyncerConfig struct {
	// BatchInterval is the time to collect missing keys before sending a request.
	BatchInterval time.Duration

	// DeduplicationInterval is the minimum time between requesting the same key.
	DeduplicationInterval time.Duration

	// RetryTimeout is the time to wait before retrying a request.
	RetryTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts per key.
	MaxRetries int

	// JitterMax is the maximum random jitter added to batch requests.
	JitterMax time.Duration

	// PublicKey is this node's public key for identifying ourselves in requests.
	PublicKey ed25519.PublicKey
}

// applyDefaults fills in default values for unset configuration fields.
func (c *BPTSyncerConfig) applyDefaults() {
	if c.BatchInterval <= 0 {
		c.BatchInterval = DefaultBPTSyncBatchInterval
	}
	if c.DeduplicationInterval <= 0 {
		c.DeduplicationInterval = DefaultBPTSyncDeduplicationInterval
	}
	if c.RetryTimeout <= 0 {
		c.RetryTimeout = DefaultBPTSyncRetryTimeout
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultBPTSyncMaxRetries
	}
	if c.JitterMax <= 0 {
		c.JitterMax = DefaultBPTSyncJitterMax
	}
}

// BPTSyncer handles requesting missing BPT entries from peers.
// It implements batching, deduplication, and retry logic.
type BPTSyncer struct {
	config BPTSyncerConfig
	store  BPTStore
	gossip *gossip.GossipLayer

	// Track in-flight requests to avoid duplicates
	inFlight   map[[32]byte]*bptInFlightRequest
	inFlightMu sync.Mutex

	// Batch pending key requests
	batchQueue   [][32]byte
	batchQueueMu sync.Mutex
	batchTimer   *time.Timer

	// Request ID counter
	nextReqID atomic.Uint64

	// Callback for received entries
	onEntryReceived func(keyHash [32]byte, value []byte)

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	requestsSent  atomic.Uint64
	responsesRecv atomic.Uint64
	entriesRecv   atomic.Uint64
}

// bptInFlightRequest tracks a pending sync request.
type bptInFlightRequest struct {
	keyHash   [32]byte
	requestID uint64
	sentAt    time.Time
	retries   int
}

// NewBPTSyncer creates a new BPTSyncer.
func NewBPTSyncer(
	config BPTSyncerConfig,
	store BPTStore,
	g *gossip.GossipLayer,
) *BPTSyncer {
	config.applyDefaults()

	return &BPTSyncer{
		config:   config,
		store:    store,
		gossip:   g,
		inFlight: make(map[[32]byte]*bptInFlightRequest),
	}
}

// SetEntryReceivedCallback sets the callback to invoke when an entry is received.
func (s *BPTSyncer) SetEntryReceivedCallback(cb func(keyHash [32]byte, value []byte)) {
	s.onEntryReceived = cb
}

// Start begins the BPT syncer's main loop.
func (s *BPTSyncer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start handlers
	s.wg.Add(2)
	go s.handleSyncRequests()
	go s.handleSyncResponses()

	// Start retry loop
	s.wg.Add(1)
	go s.retryLoop()

	slog.Info("BPTSyncer started")
	return nil
}

// Stop stops the BPT syncer.
func (s *BPTSyncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	slog.Info("BPTSyncer stopped",
		"requestsSent", s.requestsSent.Load(),
		"responsesRecv", s.responsesRecv.Load(),
		"entriesRecv", s.entriesRecv.Load())
}

// RequestMissing queues a request for missing BPT entries.
// Requests are batched and deduplicated before being sent.
func (s *BPTSyncer) RequestMissing(keyHashes [][32]byte) {
	if len(keyHashes) == 0 {
		return
	}

	s.batchQueueMu.Lock()
	defer s.batchQueueMu.Unlock()

	now := time.Now()

	for _, keyHash := range keyHashes {
		// Check if we already have it
		if s.store != nil {
			has, err := s.store.HasEntry(keyHash)
			if err == nil && has {
				continue
			}
		}

		// Check deduplication
		s.inFlightMu.Lock()
		if req, exists := s.inFlight[keyHash]; exists {
			// Check if we should retry
			if now.Sub(req.sentAt) < s.config.DeduplicationInterval {
				s.inFlightMu.Unlock()
				continue
			}
			// Too many retries
			if req.retries >= s.config.MaxRetries {
				s.inFlightMu.Unlock()
				continue
			}
		}
		s.inFlightMu.Unlock()

		// Add to batch queue
		s.batchQueue = append(s.batchQueue, keyHash)
	}

	// Start batch timer if not already running
	if len(s.batchQueue) > 0 && s.batchTimer == nil {
		// Add jitter to prevent thundering herd
		jitter := time.Duration(rand.Int64N(int64(s.config.JitterMax)))
		s.batchTimer = time.AfterFunc(s.config.BatchInterval+jitter, s.sendBatchRequest)
	}
}

// sendBatchRequest sends a batch of BPT sync requests.
func (s *BPTSyncer) sendBatchRequest() {
	s.batchQueueMu.Lock()
	if len(s.batchQueue) == 0 {
		s.batchTimer = nil
		s.batchQueueMu.Unlock()
		return
	}

	// Get keys to request
	keyHashes := s.batchQueue
	s.batchQueue = nil
	s.batchTimer = nil
	s.batchQueueMu.Unlock()

	// Filter keys we still need
	now := time.Now()
	var toRequest [][32]byte

	s.inFlightMu.Lock()
	for _, keyHash := range keyHashes {
		// Skip if we now have it
		if s.store != nil {
			has, err := s.store.HasEntry(keyHash)
			if err == nil && has {
				continue
			}
		}

		// Skip if recently requested
		if req, exists := s.inFlight[keyHash]; exists {
			if now.Sub(req.sentAt) < s.config.DeduplicationInterval {
				continue
			}
			// Update retry count
			req.retries++
			req.sentAt = now
			req.requestID = s.nextReqID.Add(1)
			toRequest = append(toRequest, keyHash)
		} else {
			// New request
			reqID := s.nextReqID.Add(1)
			s.inFlight[keyHash] = &bptInFlightRequest{
				keyHash:   keyHash,
				requestID: reqID,
				sentAt:    now,
				retries:   0,
			}
			toRequest = append(toRequest, keyHash)
		}
	}
	s.inFlightMu.Unlock()

	if len(toRequest) == 0 {
		return
	}

	// Limit request size
	if len(toRequest) > gossip.MaxBPTSyncKeys {
		// Queue the overflow for next batch
		s.batchQueueMu.Lock()
		s.batchQueue = append(s.batchQueue, toRequest[gossip.MaxBPTSyncKeys:]...)
		if s.batchTimer == nil {
			jitter := time.Duration(rand.Int64N(int64(s.config.JitterMax)))
			s.batchTimer = time.AfterFunc(s.config.BatchInterval+jitter, s.sendBatchRequest)
		}
		s.batchQueueMu.Unlock()
		toRequest = toRequest[:gossip.MaxBPTSyncKeys]
	}

	// Create and send request
	req := &gossip.BPTSyncRequest{
		Keys:      toRequest,
		Requester: s.config.PublicKey,
		RequestID: s.nextReqID.Add(1),
	}

	slog.Info("Requesting missing BPT entries",
		"count", len(toRequest))

	// Skip if no gossip layer available
	if s.gossip == nil {
		return
	}

	if err := s.gossip.BroadcastBPTSyncRequest(s.ctx, req); err != nil {
		slog.Warn("Failed to broadcast BPT sync request",
			"error", err)
		return
	}

	s.requestsSent.Add(1)
}

// handleSyncRequests processes incoming sync requests from peers.
func (s *BPTSyncer) handleSyncRequests() {
	defer s.wg.Done()

	// Skip if no gossip layer
	if s.gossip == nil {
		<-s.ctx.Done()
		return
	}

	requests := s.gossip.SubscribeBPTSyncRequests()

	for {
		select {
		case <-s.ctx.Done():
			return

		case req, ok := <-requests:
			if !ok {
				return
			}
			s.handleSyncRequest(req)
		}
	}
}

// handleSyncRequest processes a single sync request.
func (s *BPTSyncer) handleSyncRequest(req *gossip.BPTSyncRequest) {
	if req == nil || len(req.Keys) == 0 {
		return
	}

	// Skip if no store
	if s.store == nil {
		return
	}

	// Build response
	var entries []gossip.BPTEntry
	var missing [][32]byte

	for _, keyHash := range req.Keys {
		value, err := s.store.GetEntry(keyHash)
		if err != nil || value == nil {
			missing = append(missing, keyHash)
			continue
		}

		entries = append(entries, gossip.BPTEntry{
			KeyHash: keyHash,
			Value:   value,
		})

		// Limit response size
		if len(entries) >= gossip.MaxBPTSyncEntries {
			// Mark remaining as missing
			for _, kh := range req.Keys[len(entries)+len(missing):] {
				missing = append(missing, kh)
			}
			break
		}
	}

	// Only respond if we have entries to send
	if len(entries) == 0 {
		return
	}

	resp := &gossip.BPTSyncResponse{
		Entries:   entries,
		RequestID: req.RequestID,
		Missing:   missing,
	}

	// Skip if no gossip layer
	if s.gossip == nil {
		return
	}

	if err := s.gossip.BroadcastBPTSyncResponse(s.ctx, resp); err != nil {
		slog.Debug("Failed to send BPT sync response",
			"error", err)
		return
	}

	slog.Debug("Sent BPT sync response",
		"entries", len(entries),
		"missing", len(missing),
		"requestID", req.RequestID)
}

// handleSyncResponses processes incoming sync responses from peers.
func (s *BPTSyncer) handleSyncResponses() {
	defer s.wg.Done()

	// Skip if no gossip layer
	if s.gossip == nil {
		<-s.ctx.Done()
		return
	}

	responses := s.gossip.SubscribeBPTSyncResponses()

	for {
		select {
		case <-s.ctx.Done():
			return

		case resp, ok := <-responses:
			if !ok {
				return
			}
			s.handleSyncResponse(resp)
		}
	}
}

// handleSyncResponse processes a single sync response.
func (s *BPTSyncer) handleSyncResponse(resp *gossip.BPTSyncResponse) {
	if resp == nil || len(resp.Entries) == 0 {
		return
	}

	s.responsesRecv.Add(1)

	for _, entry := range resp.Entries {
		// Remove from in-flight
		s.inFlightMu.Lock()
		delete(s.inFlight, entry.KeyHash)
		s.inFlightMu.Unlock()

		// Store the entry if we have a store
		if s.store != nil {
			if err := s.store.PutEntry(entry.KeyHash, entry.Value); err != nil {
				slog.Warn("Failed to store BPT entry",
					"keyHash", entry.KeyHash,
					"error", err)
				continue
			}
		}

		// Invoke callback if set
		if s.onEntryReceived != nil {
			s.onEntryReceived(entry.KeyHash, entry.Value)
		}

		s.entriesRecv.Add(1)
	}

	slog.Debug("Received BPT sync response",
		"entries", len(resp.Entries),
		"requestID", resp.RequestID)
}

// retryLoop periodically checks for timed-out requests and retries them.
func (s *BPTSyncer) retryLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.RetryTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.checkRetries()
		}
	}
}

// checkRetries finds timed-out requests and queues them for retry.
func (s *BPTSyncer) checkRetries() {
	now := time.Now()
	var toRetry [][32]byte

	s.inFlightMu.Lock()
	for keyHash, req := range s.inFlight {
		// Check if we now have it
		if s.store != nil {
			has, err := s.store.HasEntry(keyHash)
			if err == nil && has {
				delete(s.inFlight, keyHash)
				continue
			}
		}

		// Check if timed out
		if now.Sub(req.sentAt) < s.config.RetryTimeout {
			continue
		}

		// Check retry limit
		if req.retries >= s.config.MaxRetries {
			slog.Debug("Giving up on BPT sync",
				"keyHash", keyHash,
				"retries", req.retries)
			delete(s.inFlight, keyHash)
			continue
		}

		toRetry = append(toRetry, keyHash)
	}
	s.inFlightMu.Unlock()

	// Queue for retry
	if len(toRetry) > 0 {
		slog.Debug("Retrying BPT sync",
			"count", len(toRetry))
		s.RequestMissing(toRetry)
	}
}

// ClearInFlight removes a key hash from the in-flight tracking.
// This should be called when an entry is received via normal gossip
// (not through sync) to prevent stale in-flight entries.
func (s *BPTSyncer) ClearInFlight(keyHash [32]byte) {
	s.inFlightMu.Lock()
	delete(s.inFlight, keyHash)
	s.inFlightMu.Unlock()
}

// Metrics returns syncer metrics.
func (s *BPTSyncer) Metrics() (requestsSent, responsesRecv, entriesRecv uint64) {
	return s.requestsSent.Load(), s.responsesRecv.Load(), s.entriesRecv.Load()
}

// InFlightCount returns the number of in-flight sync requests.
func (s *BPTSyncer) InFlightCount() int {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	return len(s.inFlight)
}
