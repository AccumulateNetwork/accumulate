// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package worker implements the Worker component for DAG-based consensus.
// Workers collect transactions from clients, bundle them into batches,
// broadcast batch digests via gossip, and serve batch data on request.
// This is the "data availability" layer in the Narwhal/Bullshark architecture.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default configuration values.
const (
	DefaultBatchSize       = 500                    // max transactions per batch
	DefaultBatchTimeout    = 100 * time.Millisecond // max time to wait for full batch
	DefaultMaxBatchBytes   = 500 * 1024             // 500KB max batch size
	DefaultMaxPendingSize  = 10 * 1024 * 1024       // 10MB max pending transactions
	DefaultMaxStoredBatches = 1000                  // max batches stored before eviction (reduced for memory safety)
)

// ErrWorkerClosed is returned when operations are attempted on a closed worker.
var ErrWorkerClosed = errors.New("worker is closed")

// ErrBackpressure is returned when the worker cannot accept more transactions
// due to memory limits being reached (pending queue full or too many uncommitted batches).
var ErrBackpressure = errors.New("worker backpressure: pending transactions exceed limit")

// ErrValidationFailed is returned when a transaction fails pre-batch validation.
var ErrValidationFailed = errors.New("transaction validation failed")

// TransactionValidator validates transactions before they are added to a batch.
// This is equivalent to CometBFT's CheckTx.
type TransactionValidator interface {
	// ValidateTransaction validates a transaction before batching.
	// Returns nil if valid, or an error describing why the transaction is invalid.
	ValidateTransaction(tx []byte) error
}

// Config holds the configuration for a Worker.
type Config struct {
	// ID is the unique identifier for this worker (0-255).
	ID types.WorkerID

	// Partition is the network partition this worker operates on.
	Partition string

	// BatchSize is the maximum number of transactions per batch.
	// When this limit is reached, a batch is created immediately.
	// Defaults to DefaultBatchSize.
	BatchSize int

	// BatchTimeout is the maximum time to wait for a full batch.
	// A batch is created when this timeout fires, even if not full.
	// Defaults to DefaultBatchTimeout.
	BatchTimeout time.Duration

	// MaxBatchBytes is the maximum size of a batch in bytes.
	// When this limit is reached, a batch is created immediately.
	// Defaults to DefaultMaxBatchBytes.
	MaxBatchBytes int

	// MaxPendingSize is the maximum total size of pending transactions.
	// When exceeded, Submit will return ErrBackpressure.
	// Defaults to DefaultMaxPendingSize.
	MaxPendingSize int

	// MaxStoredBatches is the maximum number of batches to store.
	// When exceeded, random batches are evicted to make room.
	// This prevents unbounded memory growth from gossip batches.
	// Defaults to DefaultMaxStoredBatches.
	MaxStoredBatches int

	// Validator validates transactions before they are added to a batch.
	// If nil, no validation is performed (not recommended for production).
	Validator TransactionValidator
}

// applyDefaults fills in default values for unset configuration fields.
func (c *Config) applyDefaults() {
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.BatchTimeout <= 0 {
		c.BatchTimeout = DefaultBatchTimeout
	}
	if c.MaxBatchBytes <= 0 {
		c.MaxBatchBytes = DefaultMaxBatchBytes
	}
	if c.MaxPendingSize <= 0 {
		c.MaxPendingSize = DefaultMaxPendingSize
	}
	if c.MaxStoredBatches <= 0 {
		c.MaxStoredBatches = DefaultMaxStoredBatches
	}
}

// BatchStore defines the interface for storing and retrieving batches.
// This allows the worker to serve as a batch store for the gossip layer.
type BatchStore interface {
	// GetBatch retrieves a batch by its digest.
	// Returns nil, nil if the batch is not found.
	GetBatch(digest types.BatchDigest) (*types.Batch, error)

	// StoreBatch stores a batch.
	StoreBatch(batch *types.Batch) error
}

// Worker collects transactions and creates batches for the consensus layer.
// It implements the "data availability" layer in the Narwhal/Bullshark architecture.
type Worker struct {
	config    Config
	gossip    *gossip.GossipLayer
	validator TransactionValidator

	// Pending transactions
	mu          sync.Mutex
	pending     [][]byte
	pendingSize int

	// Batch storage
	batchMu sync.RWMutex
	batches map[types.BatchDigest]*types.Batch

	// Available batch digests (for header creation)
	availableMu      sync.Mutex
	availableBatches []types.BatchDigest

	// Lifecycle management
	lifecycleMu sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	started     bool
	closed      atomic.Bool

	// Metrics
	batchesCreated   atomic.Uint64
	txnsProcessed    atomic.Uint64
	txnsReceived     atomic.Uint64
	txnsValidated    atomic.Uint64
	txnsRejected     atomic.Uint64

	// Trigger channel for immediate batch creation
	triggerBatch chan struct{}
}

// New creates a new Worker with the given configuration and gossip layer.
func New(config Config, g *gossip.GossipLayer) *Worker {
	config.applyDefaults()

	return &Worker{
		config:           config,
		gossip:           g,
		validator:        config.Validator,
		pending:          make([][]byte, 0, config.BatchSize),
		batches:          make(map[types.BatchDigest]*types.Batch),
		availableBatches: make([]types.BatchDigest, 0),
		triggerBatch:     make(chan struct{}, 1),
	}
}

// Submit adds a transaction to the pending batch.
// Returns ErrBackpressure if the worker cannot accept more transactions.
// Returns ErrWorkerClosed if the worker has been closed.
// Returns ErrValidationFailed (wrapped) if the transaction fails validation.
func (w *Worker) Submit(tx []byte) error {
	if w.closed.Load() {
		return ErrWorkerClosed
	}

	if len(tx) == 0 {
		return errors.New("transaction is empty")
	}

	w.txnsReceived.Add(1)

	// Validate transaction before adding to pending batch (CheckTx equivalent)
	if w.validator != nil {
		if err := w.validator.ValidateTransaction(tx); err != nil {
			w.txnsRejected.Add(1)
			slog.Debug("Transaction validation failed",
				"error", err,
				"workerID", w.config.ID)
			return fmt.Errorf("%w: %v", ErrValidationFailed, err)
		}
		w.txnsValidated.Add(1)
	}

	// Check batch count backpressure first (no lock needed, just read)
	w.batchMu.RLock()
	batchCount := len(w.batches)
	w.batchMu.RUnlock()
	if batchCount >= w.config.MaxStoredBatches {
		return ErrBackpressure
	}

	w.mu.Lock()

	// Check pending size backpressure
	if w.pendingSize+len(tx) > w.config.MaxPendingSize {
		w.mu.Unlock()
		return ErrBackpressure
	}

	// Copy the transaction to avoid external modification
	txCopy := make([]byte, len(tx))
	copy(txCopy, tx)

	w.pending = append(w.pending, txCopy)
	w.pendingSize += len(txCopy)

	// Check if we should create a batch immediately
	shouldTrigger := len(w.pending) >= w.config.BatchSize || w.pendingSize >= w.config.MaxBatchBytes

	w.mu.Unlock()

	// Trigger batch creation outside the lock
	if shouldTrigger {
		select {
		case w.triggerBatch <- struct{}{}:
		default:
			// Channel already has a pending trigger
		}
	}

	return nil
}

// Start begins the worker's batch creation loop and incoming batch handler.
// This method blocks until the context is canceled or Close is called.
func (w *Worker) Start(ctx context.Context) error {
	if w.closed.Load() {
		return ErrWorkerClosed
	}

	w.lifecycleMu.Lock()
	if w.started {
		w.lifecycleMu.Unlock()
		return errors.New("worker already started")
	}
	w.started = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	workerCtx := w.ctx
	w.lifecycleMu.Unlock()

	// Start the batch creation loop
	w.wg.Add(1)
	go w.batchLoop()

	// Start the incoming batch handler
	w.wg.Add(1)
	go w.handleIncomingBatches()

	slog.Info("Worker started",
		"id", w.config.ID,
		"partition", w.config.Partition,
		"batchSize", w.config.BatchSize,
		"batchTimeout", w.config.BatchTimeout,
		"maxBatchBytes", w.config.MaxBatchBytes)

	// Wait for context cancellation
	<-workerCtx.Done()

	return nil
}

// Close stops the worker and releases resources.
func (w *Worker) Close() error {
	if w.closed.Swap(true) {
		return nil // Already closed
	}

	w.lifecycleMu.Lock()
	cancelFn := w.cancel
	w.lifecycleMu.Unlock()

	if cancelFn != nil {
		cancelFn()
	}

	// Wait for goroutines to finish
	w.wg.Wait()

	slog.Info("Worker closed",
		"id", w.config.ID,
		"batchesCreated", w.batchesCreated.Load(),
		"txnsProcessed", w.txnsProcessed.Load())

	return nil
}

// GetBatch retrieves a batch by its digest.
// Returns nil, nil if the batch is not found.
// Implements the BatchStore interface.
func (w *Worker) GetBatch(digest types.BatchDigest) (*types.Batch, error) {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()

	batch, ok := w.batches[digest]
	if !ok {
		return nil, nil // Not found, not an error
	}

	return batch, nil
}

// StoreBatch stores a batch received from another worker.
// Implements the BatchStore interface.
func (w *Worker) StoreBatch(batch *types.Batch) error {
	if batch == nil {
		return errors.New("batch is nil")
	}

	digest := batch.Digest()

	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	// Only store if we don't already have it
	if _, exists := w.batches[digest]; !exists {
		// Evict random batches if we're at the limit
		// This prevents unbounded memory growth from gossip batches
		if len(w.batches) >= w.config.MaxStoredBatches {
			evictCount := len(w.batches) / 10 // Evict 10%
			if evictCount < 1 {
				evictCount = 1
			}
			evicted := 0
			for d := range w.batches {
				delete(w.batches, d)
				evicted++
				if evicted >= evictCount {
					break
				}
			}
			slog.Debug("Evicted batches due to storage limit",
				"evicted", evicted,
				"remaining", len(w.batches))
		}
		w.batches[digest] = batch
	}

	return nil
}

// AvailableBatches returns all batch digests that are available for header creation.
// These are batches created by this worker that have been broadcast but not yet
// included in a committed header.
func (w *Worker) AvailableBatches() []types.BatchDigest {
	w.availableMu.Lock()
	defer w.availableMu.Unlock()

	// Return a copy to prevent external modification
	result := make([]types.BatchDigest, len(w.availableBatches))
	copy(result, w.availableBatches)
	return result
}

// ConsumeAvailableBatches returns and clears all available batch digests.
// This is used by the primary when creating a header.
func (w *Worker) ConsumeAvailableBatches() []types.BatchDigest {
	w.availableMu.Lock()
	defer w.availableMu.Unlock()

	result := w.availableBatches
	w.availableBatches = make([]types.BatchDigest, 0)
	return result
}

// PruneBatches removes batches that have been committed to consensus.
// This should be called after batches are finalized to free memory.
func (w *Worker) PruneBatches(committed []types.BatchDigest) {
	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	for _, digest := range committed {
		delete(w.batches, digest)
	}

	slog.Debug("Pruned committed batches",
		"count", len(committed),
		"remaining", len(w.batches))
}

// PendingCount returns the number of pending transactions.
func (w *Worker) PendingCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending)
}

// PendingSize returns the total size of pending transactions in bytes.
func (w *Worker) PendingSize() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pendingSize
}

// BatchCount returns the number of stored batches.
func (w *Worker) BatchCount() int {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	return len(w.batches)
}

// Metrics returns the worker's metrics.
func (w *Worker) Metrics() (batchesCreated, txnsProcessed, txnsReceived uint64) {
	return w.batchesCreated.Load(), w.txnsProcessed.Load(), w.txnsReceived.Load()
}

// ValidationMetrics returns the worker's validation metrics.
func (w *Worker) ValidationMetrics() (validated, rejected uint64) {
	return w.txnsValidated.Load(), w.txnsRejected.Load()
}

// ID returns the worker's ID.
func (w *Worker) ID() types.WorkerID {
	return w.config.ID
}

// Partition returns the partition this worker operates on.
func (w *Worker) Partition() string {
	return w.config.Partition
}

// batchLoop runs the batch creation loop.
// Batches are created when:
// 1. Transaction count reaches BatchSize
// 2. Batch byte size reaches MaxBatchBytes
// 3. Timeout fires (BatchTimeout)
func (w *Worker) batchLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			// Create final batch from any remaining transactions
			w.createAndBroadcastBatch()
			return

		case <-ticker.C:
			// Timeout - create batch if we have pending transactions
			w.createAndBroadcastBatch()

		case <-w.triggerBatch:
			// Size limit reached - create batch immediately
			w.createAndBroadcastBatch()
			// Reset the ticker to avoid creating another batch too soon
			ticker.Reset(w.config.BatchTimeout)
		}
	}
}

// createAndBroadcastBatch creates a batch from pending transactions and broadcasts it.
func (w *Worker) createAndBroadcastBatch() {
	batch := w.createBatch()
	if batch == nil {
		return
	}

	// Store locally first (with eviction to prevent unbounded growth)
	digest := batch.Digest()
	w.batchMu.Lock()
	if len(w.batches) >= w.config.MaxStoredBatches {
		evictCount := len(w.batches) / 10 // Evict 10%
		if evictCount < 1 {
			evictCount = 1
		}
		evicted := 0
		for d := range w.batches {
			delete(w.batches, d)
			evicted++
			if evicted >= evictCount {
				break
			}
		}
		slog.Warn("Evicted local batches due to storage limit (consensus not keeping up)",
			"evicted", evicted,
			"remaining", len(w.batches))
	}
	w.batches[digest] = batch
	w.batchMu.Unlock()

	// Add to available batches
	w.availableMu.Lock()
	w.availableBatches = append(w.availableBatches, digest)
	w.availableMu.Unlock()

	// Update metrics
	w.batchesCreated.Add(1)
	w.txnsProcessed.Add(uint64(batch.Len()))

	// Broadcast to network
	if w.gossip != nil {
		if err := w.gossip.BroadcastBatch(w.ctx, batch); err != nil {
			slog.Warn("Failed to broadcast batch",
				"error", err,
				"digest", digest.String(),
				"txns", batch.Len())
		} else {
			slog.Debug("Broadcast batch",
				"digest", digest.String(),
				"txns", batch.Len(),
				"size", batch.Size())
		}
	}
}

// createBatch creates a new batch from pending transactions.
// Returns nil if there are no pending transactions.
func (w *Worker) createBatch() *types.Batch {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.pending) == 0 {
		return nil
	}

	// Take all pending transactions
	transactions := w.pending
	w.pending = make([][]byte, 0, w.config.BatchSize)
	w.pendingSize = 0

	return types.NewBatch(transactions)
}

// handleIncomingBatches listens for batches from other workers and stores them.
func (w *Worker) handleIncomingBatches() {
	defer w.wg.Done()

	if w.gossip == nil {
		return
	}

	batches := w.gossip.SubscribeBatches()
	for {
		select {
		case <-w.ctx.Done():
			return

		case batch, ok := <-batches:
			if !ok {
				return // Channel closed
			}

			if batch != nil {
				if err := w.StoreBatch(batch); err != nil {
					slog.Warn("Failed to store incoming batch",
						"error", err,
						"digest", batch.Digest().String())
				} else {
					slog.Debug("Stored incoming batch",
						"digest", batch.Digest().String(),
						"txns", batch.Len())
				}
			}
		}
	}
}

// HasBatch returns true if the worker has the batch with the given digest.
func (w *Worker) HasBatch(digest types.BatchDigest) bool {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	_, ok := w.batches[digest]
	return ok
}

// BatchDigests returns the digests of all stored batches.
func (w *Worker) BatchDigests() []types.BatchDigest {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()

	digests := make([]types.BatchDigest, 0, len(w.batches))
	for digest := range w.batches {
		digests = append(digests, digest)
	}
	return digests
}

// String returns a string representation of the worker.
func (w *Worker) String() string {
	return fmt.Sprintf("Worker{id=%d, partition=%s}", w.config.ID, w.config.Partition)
}
