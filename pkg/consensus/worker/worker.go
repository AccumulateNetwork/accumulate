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
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default configuration values.
const (
	DefaultBatchSize         = 500                    // max transactions per batch
	DefaultBatchTimeout      = 100 * time.Millisecond // max time to wait for full batch
	DefaultMaxBatchBytes     = 500 * 1024             // 500KB max batch size
	DefaultMaxPendingSize    = 10 * 1024 * 1024       // 10MB max pending transactions
	DefaultMaxPendingCount   = 10000                  // max pending transaction count
	DefaultMaxStoredBatches  = 1000                   // max batches stored before eviction (reduced for memory safety)
	DefaultMaxBatchQueueSize = 1000                   // max batches in available queue before blocking
)

// ErrWorkerClosed is returned when operations are attempted on a closed worker.
var ErrWorkerClosed = errors.New("worker is closed")

// ErrBackpressure is returned when the worker cannot accept more transactions
// due to memory limits being reached (pending queue full or too many uncommitted batches).
var ErrBackpressure = errors.New("worker backpressure: pending transactions exceed limit")

// ErrTransactionTooLarge is returned when a single transaction exceeds the
// batch byte limit. A distinct error, not backpressure: backpressure clears
// on retry, this never will — the transaction cannot fit in any batch, and
// accepting it quietly distorted batching instead of refusing it (#4141).
var ErrTransactionTooLarge = errors.New("transaction exceeds the batch size limit")

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

	// MaxPendingSize is the maximum total size of pending transactions in bytes.
	// When exceeded, Submit will return ErrBackpressure.
	// Defaults to DefaultMaxPendingSize.
	MaxPendingSize int

	// MaxPendingCount is the maximum number of pending transactions.
	// When exceeded, Submit will return ErrBackpressure.
	// Defaults to DefaultMaxPendingCount.
	MaxPendingCount int

	// MaxStoredBatches is the maximum number of batches to store.
	// When exceeded, random batches are evicted to make room.
	// This prevents unbounded memory growth from gossip batches.
	// Defaults to DefaultMaxStoredBatches.
	MaxStoredBatches int

	// ReproposeAfter is how long an own batch may sit uncommitted before it
	// is proposed again; ReproposeTick is how often the scan runs. Defaults
	// to DefaultReproposeAfter / DefaultReproposeTick.
	ReproposeAfter time.Duration
	ReproposeTick  time.Duration

	// MaxBatchQueueSize is the maximum number of batches in the available queue.
	// When exceeded, batch creation will block until space is available.
	// This provides backpressure when consensus cannot keep up.
	// Defaults to DefaultMaxBatchQueueSize.
	MaxBatchQueueSize int

	// Validator validates transactions before they are added to a batch.
	// If nil, no validation is performed (not recommended for production).
	Validator TransactionValidator

	// MaxTombstones is how many batch removals the worker remembers, so that
	// a batch a certificate cannot find can still say why it is gone.
	// Defaults to DefaultMaxTombstones. Negative disables the record.
	MaxTombstones int

	// RetainCommittedFor is how long a committed batch stays fetchable for
	// peers that fell behind, and MaxRetainedBatches caps how many are held.
	// Defaults to DefaultRetainCommittedFor / DefaultMaxRetainedBatches.
	// Negative disables retention, restoring the old delete-on-commit
	// behaviour — which strands any node that misses the commit (#4128).
	RetainCommittedFor time.Duration
	MaxRetainedBatches int
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
	if c.MaxPendingCount <= 0 {
		c.MaxPendingCount = DefaultMaxPendingCount
	}
	if c.MaxStoredBatches <= 0 {
		c.MaxStoredBatches = DefaultMaxStoredBatches
	}
	if c.MaxBatchQueueSize <= 0 {
		c.MaxBatchQueueSize = DefaultMaxBatchQueueSize
	}
	if c.ReproposeAfter <= 0 {
		c.ReproposeAfter = DefaultReproposeAfter
	}
	if c.ReproposeTick <= 0 {
		c.ReproposeTick = DefaultReproposeTick
	}
	if c.MaxTombstones == 0 {
		c.MaxTombstones = DefaultMaxTombstones
	}
	if c.RetainCommittedFor == 0 {
		c.RetainCommittedFor = DefaultRetainCommittedFor
	}
	if c.MaxRetainedBatches == 0 {
		c.MaxRetainedBatches = DefaultMaxRetainedBatches
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

// lruEntry wraps a batch with its position in the LRU list.
type lruEntry struct {
	batch   *types.Batch
	element *list.Element // pointer to element in lruList

	// own marks batches this worker created (as opposed to copies received
	// via gossip). Only own batches are re-proposed — the author is
	// responsible for its batches reaching a committed certificate.
	own bool
	// lastQueued is when the digest was last placed on the availability
	// queue. A batch still stored (= not pruned = not committed) long after
	// it was last proposed has fallen out of the committed leaders' causal
	// history and must be proposed again.
	lastQueued time.Time
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

	// Batch storage with LRU eviction
	batchMu sync.RWMutex
	batches map[types.BatchDigest]*lruEntry
	lruList *list.List // LRU tracking: front = most recent, back = least recent

	// Tombstones: why a batch left the store. A committed certificate whose
	// batch is absent everywhere halts the partition permanently (#4125), and
	// without this the log cannot say whether it was pruned, evicted, or never
	// held. Bounded by maxTombstones. Guarded by batchMu.
	gone          map[types.BatchDigest]BatchGone
	goneOrder     []types.BatchDigest
	maxTombstones int

	// Committed batches kept fetchable for peers that fell behind (#4128).
	// Separate from `batches` on purpose: `batches` is what this node still
	// owes consensus, `retained` is what it can still serve. Guarded by
	// batchMu.
	retained      map[types.BatchDigest]*retainedBatch
	retainedOrder []types.BatchDigest
	maxRetained   int
	retainFor     time.Duration

	// Available batch digests (for header creation) - bounded queue with backpressure
	availableBatchQueue chan types.BatchDigest
	queueDepth          atomic.Int64
	batchesBlocked      atomic.Uint64

	// Lifecycle management
	lifecycleMu sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	started     bool
	closed      atomic.Bool

	// Metrics
	batchesCreated atomic.Uint64
	txnsProcessed  atomic.Uint64
	txnsReceived   atomic.Uint64
	txnsValidated  atomic.Uint64
	txnsRejected   atomic.Uint64

	// Trigger channel for immediate batch creation
	triggerBatch chan struct{}

	// Eviction control
	triggerEviction chan struct{}
}

// New creates a new Worker with the given configuration and gossip layer.
func New(config Config, g *gossip.GossipLayer) *Worker {
	config.applyDefaults()

	return &Worker{
		config:              config,
		gossip:              g,
		validator:           config.Validator,
		pending:             make([][]byte, 0, config.BatchSize),
		batches:             make(map[types.BatchDigest]*lruEntry),
		lruList:             list.New(),
		gone:                make(map[types.BatchDigest]BatchGone),
		maxTombstones:       config.MaxTombstones,
		retained:            make(map[types.BatchDigest]*retainedBatch),
		maxRetained:         config.MaxRetainedBatches,
		retainFor:           config.RetainCommittedFor,
		availableBatchQueue: make(chan types.BatchDigest, config.MaxBatchQueueSize),
		triggerBatch:        make(chan struct{}, 1),
		triggerEviction:     make(chan struct{}, 1),
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

	// A transaction that cannot fit in ANY batch is refused up front, visibly
	// (#4141). Backpressure below is retryable; this is not. The refusal
	// counts in the metrics — an invisible refusal reads as traffic that
	// never arrived (#4151).
	if len(tx) > w.config.MaxBatchBytes {
		w.txnsReceived.Add(1)
		w.txnsRejected.Add(1)
		return fmt.Errorf("%w: %d bytes, limit %d", ErrTransactionTooLarge, len(tx), w.config.MaxBatchBytes)
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

	if txTraceEnabled {
		slog.Info("TX accepted", "tx", txID(tx), "worker", w.config.ID,
			"partition", w.config.Partition, "bytes", len(tx))
	}

	w.mu.Lock()

	// Check pending count backpressure
	if len(w.pending) >= w.config.MaxPendingCount {
		w.mu.Unlock()
		return ErrBackpressure
	}

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

	// Start the LRU eviction loop
	w.wg.Add(1)
	go w.evictionLoop()

	// Start the re-proposal loop
	w.wg.Add(1)
	go w.reproposeLoop()

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
// Updates LRU on access to mark the batch as recently used.
func (w *Worker) GetBatch(digest types.BatchDigest) (*types.Batch, error) {
	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	entry, ok := w.batches[digest]
	if !ok {
		// Committed and no longer active, but possibly still retained for
		// peers catching up. Retention has no LRU position to update.
		if b := w.getRetained(digest); b != nil {
			return b, nil
		}
		return nil, nil // Not found, not an error
	}

	// Move to front of LRU list (most recently used)
	w.lruList.MoveToFront(entry.element)

	return entry.batch, nil
}

// StoreBatch stores a batch received from another worker.
// Implements the BatchStore interface.
// Uses LRU eviction policy when storage limit is reached.
func (w *Worker) StoreBatch(batch *types.Batch) error {
	if batch == nil {
		return errors.New("batch is nil")
	}

	digest := batch.Digest()

	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	// If batch already exists, move it to front (mark as recently used)
	if entry, exists := w.batches[digest]; exists {
		w.lruList.MoveToFront(entry.element)
		return nil
	}

	// Trigger eviction if we're approaching the limit (non-blocking)
	// Eviction is handled by dedicated goroutine to minimize lock contention
	if len(w.batches) > w.config.MaxStoredBatches {
		select {
		case w.triggerEviction <- struct{}{}:
		default:
			// Eviction already triggered or in progress
		}
	}

	// Add new batch to front of LRU list (most recently used)
	element := w.lruList.PushFront(digest)
	w.batches[digest] = &lruEntry{
		batch:   batch,
		element: element,
	}

	return nil
}

// AvailableBatches returns all batch digests that are available for header creation.
// These are batches created by this worker that have been broadcast but not yet
// included in a committed header. This method peeks without consuming.
func (w *Worker) AvailableBatches() []types.BatchDigest {
	var result []types.BatchDigest

	// Drain all available batches from channel without blocking
	for {
		select {
		case digest := <-w.availableBatchQueue:
			result = append(result, digest)
		default:
			// No more batches available
			w.queueDepth.Store(0)
			return result
		}
	}
}

// ConsumeAvailableBatches returns and clears all available batch digests.
// This is used by the primary when creating a header.
func (w *Worker) ConsumeAvailableBatches() []types.BatchDigest {
	var result []types.BatchDigest

	// Drain all available batches from channel without blocking
	for {
		select {
		case digest := <-w.availableBatchQueue:
			result = append(result, digest)
			w.queueDepth.Add(-1)
		default:
			// No more batches available
			w.queueDepth.Store(0)
			return result
		}
	}
}

// RequeueBatches makes batch digests available for header creation again.
// Used when a header is discarded without ever becoming a certificate: its
// batches were consumed by the header and never committed, and without a
// requeue every transaction inside them is silently lost. Digests whose batch
// data is no longer stored (committed elsewhere and pruned, or LRU-evicted)
// are skipped — there is nothing left to deliver.
func (w *Worker) RequeueBatches(digests []types.BatchDigest) {
	for _, digest := range digests {
		w.batchMu.Lock()
		entry, ok := w.batches[digest]
		if ok {
			entry.lastQueued = time.Now()
		}
		w.batchMu.Unlock()
		if !ok {
			continue
		}
		select {
		case w.availableBatchQueue <- digest:
			w.queueDepth.Add(1)
		default:
			slog.Warn("Requeue dropped batch — availability queue full",
				"digest", digest.String(), "worker", w.ID())
		}
	}
}

// PruneBatches removes batches that have been committed to consensus.
// This should be called after batches are finalized to free memory.
// Also removes entries from the LRU list.
func (w *Worker) PruneBatches(committed []types.BatchDigest) {
	w.PruneCommitted(committed, CommitInfo{})
}

// PruneBatchesAt is PruneCommitted with only a human-readable detail.
func (w *Worker) PruneBatchesAt(committed []types.BatchDigest, detail string) {
	w.PruneCommitted(committed, CommitInfo{Detail: detail})
}

// CommitInfo identifies the commit that retired a set of batches.
type CommitInfo struct {
	// Cert is the committing certificate's digest. Recorded so that a later
	// delivery of the SAME certificate can be recognised as a re-delivery
	// rather than waited on forever (#4125).
	Cert string
	// Detail is for humans: block, round, author.
	Detail string
}

// PruneCommitted retires batches that a certificate committed. They leave the
// active store — so they stop being re-proposed — and enter the retention
// window, where they stay fetchable for peers that have not caught up yet.
func (w *Worker) PruneCommitted(committed []types.BatchDigest, info CommitInfo) {
	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	pruned := 0
	for _, digest := range committed {
		if entry, ok := w.batches[digest]; ok {
			w.lruList.Remove(entry.element)
			delete(w.batches, digest)
			// Committed, so it leaves the active store and stops being
			// re-proposed — but keep it fetchable for a while, because a peer
			// that missed this commit has nowhere else to get it (#4128).
			w.retain(digest, entry.batch, info.Detail, info.Cert)
			w.noteGone(digest, GonePruned, info.Detail, info.Cert)
			pruned++
		}
	}

	slog.Debug("Pruned committed batches",
		"count", len(committed),
		"pruned", pruned,
		"detail", info.Detail,
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

// QueueMetrics returns the worker's batch queue metrics.
// queueDepth is the current number of batches in the available queue.
// batchesBlocked is the total number of times batch creation was blocked due to full queue.
func (w *Worker) QueueMetrics() (queueDepth int64, batchesBlocked uint64) {
	return w.queueDepth.Load(), w.batchesBlocked.Load()
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

	// Store locally first (eviction is handled by dedicated goroutine)
	digest := batch.Digest()
	w.batchMu.Lock()
	// Trigger eviction if we're approaching the limit (non-blocking)
	if len(w.batches) > w.config.MaxStoredBatches {
		select {
		case w.triggerEviction <- struct{}{}:
		default:
			// Eviction already triggered or in progress
		}
	}
	// Add new batch to front of LRU list (most recently used)
	element := w.lruList.PushFront(digest)
	w.batches[digest] = &lruEntry{
		batch:      batch,
		element:    element,
		own:        true,
		lastQueued: time.Now(),
	}
	w.batchMu.Unlock()

	// Update metrics immediately after creating the batch
	// This must happen before enqueueing to ensure metrics are updated even if shutdown occurs
	w.batchesCreated.Add(1)
	w.txnsProcessed.Add(uint64(batch.Len()))

	if txTraceEnabled {
		slog.Info("TX batched", "batch", digest.String()[:12],
			"worker", w.config.ID, "partition", w.config.Partition,
			"txs", batch.Len(), "ids", strings.Join(txIDs(batch.Transactions), ","))
	}

	// Add to available batch queue (blocking backpressure if full)
	queueDepth := w.queueDepth.Add(1)

	// Log if we're about to block due to full queue
	if queueDepth > int64(w.config.MaxBatchQueueSize) {
		w.batchesBlocked.Add(1)
		slog.Warn("Batch queue full, applying backpressure",
			"queueDepth", queueDepth,
			"maxQueueSize", w.config.MaxBatchQueueSize,
			"workerID", w.config.ID,
			"partition", w.config.Partition)
	}

	// This will block if the queue is full (backpressure)
	select {
	case w.availableBatchQueue <- digest:
		// Successfully enqueued
	case <-w.ctx.Done():
		// Worker is shutting down
		w.queueDepth.Add(-1)
		return
	}

	// Warn if batch queue depth is excessive (consensus not consuming batches fast enough)
	if queueDepth > 500 {
		slog.Warn("Batch queue depth exceeds threshold",
			"queueDepth", queueDepth,
			"workerID", w.config.ID,
			"partition", w.config.Partition)
	}

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

// DefaultReproposeAfter is how long an own batch may sit uncommitted before
// it is proposed again; DefaultReproposeTick is how often the scan runs.
const DefaultReproposeAfter = 15 * time.Second
const DefaultReproposeTick = 5 * time.Second

// reproposeLoop re-queues own batches that were proposed but never committed.
//
// Consensus only executes certificates that land in a committed leader's
// causal history. A header can certify and still fall outside every committed
// leader's parent graph — in run 20260820T084028Z 46% of certificates
// (5,343 of 9,900 committed) did, and every transaction in their batches
// silently vanished; that loss is what starved anchor-signature thresholds
// across partitions (#4111). Narwhal's delivery guarantee is at the BATCH
// level: a batch must be proposed again until it commits. PruneBatches is the
// commit signal — a batch still stored long after it was last queued has
// fallen out of history and goes back on the availability queue. Execution is
// idempotent (already-delivered messages are skipped), so the rare batch that
// commits twice costs bytes, not correctness.
func (w *Worker) reproposeLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.ReproposeTick)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
		}

		var stale []types.BatchDigest
		now := time.Now()
		w.batchMu.Lock()
		for digest, entry := range w.batches {
			if entry.own && now.Sub(entry.lastQueued) > w.config.ReproposeAfter {
				stale = append(stale, digest)
				entry.lastQueued = now
			}
		}
		w.batchMu.Unlock()

		requeued := 0
		for _, digest := range stale {
			select {
			case w.availableBatchQueue <- digest:
				w.queueDepth.Add(1)
				requeued++
			default:
				// Queue full — lastQueued is already stamped, so this batch
				// waits a full reproposeAfter before the next attempt. With
				// the queue this backed up there is plenty in flight already.
			}
		}
		if requeued > 0 {
			slog.Info("Re-proposed uncommitted batches",
				"count", requeued, "worker", w.config.ID,
				"partition", w.config.Partition)
		}
	}
}

// evictionLoop runs the LRU eviction process in a dedicated goroutine.
// This minimizes lock contention by moving eviction out of the critical path
// of batch creation and storage operations.
func (w *Worker) evictionLoop() {
	defer w.wg.Done()

	// Run eviction checks periodically and when triggered
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return

		case <-ticker.C:
			w.performEviction()
			w.sweepRetained()

		case <-w.triggerEviction:
			w.performEviction()
		}
	}
}

// performEviction evicts LRU batches if the storage limit is exceeded.
// This is called by the dedicated goroutine to minimize lock contention.
func (w *Worker) performEviction() {
	w.batchMu.Lock()
	defer w.batchMu.Unlock()

	if len(w.batches) <= w.config.MaxStoredBatches {
		return // No eviction needed unless we exceed the limit
	}

	// Evict just enough to get back to MaxStoredBatches
	// Plus a small safety margin (10% of max) to reduce eviction frequency
	targetCount := int(float64(w.config.MaxStoredBatches) * 1.1)
	evictCount := len(w.batches) - targetCount
	if evictCount < 1 {
		evictCount = 1
	}

	// Never evict a batch this worker AUTHORED and has not yet seen committed.
	// Bullshark commits leaders in causal order, so an early leader can be
	// committed thousands of rounds late; if the author has already
	// LRU-evicted that leader's batch, CollectBatches finds it nowhere
	// (absence=no-record, peerHits=0) and the partition wedges permanently
	// (#4159). Gossiped copies are just cache and stay evictable — the author
	// is the source of truth and must retain its own batches until PruneCommitted
	// moves them to `retained`. Walk from the LRU back (least-recently-used)
	// toward the front, skipping own entries.
	evicted, skippedOwn := 0, 0
	for e := w.lruList.Back(); e != nil && evicted < evictCount; {
		prev := e.Prev()
		lruDigest := e.Value.(types.BatchDigest)
		if entry, ok := w.batches[lruDigest]; ok && entry.own {
			skippedOwn++
			e = prev
			continue
		}
		w.lruList.Remove(e)
		delete(w.batches, lruDigest)
		w.noteGone(lruDigest, GoneEvicted,
			fmt.Sprintf("store over limit %d", w.config.MaxStoredBatches), "")
		evicted++
		e = prev
	}

	if evicted > 0 {
		slog.Warn("Evicted batches due to storage limit (LRU)",
			"evicted", evicted,
			"remaining", len(w.batches),
			"skippedOwnUncommitted", skippedOwn,
			"workerID", w.config.ID)
	}
	// Could not reach the target because our own uncommitted batches are not
	// evictable: the store is growing because OUR batches are not committing.
	// That is the pressure that #4159 turned into a permanent wedge when these
	// batches were silently dropped; surface it rather than lose data a late
	// commit will need.
	if evicted < evictCount && skippedOwn > 0 {
		slog.Warn("Batch store over limit with un-evictable own uncommitted batches (commit is lagging)",
			"stored", len(w.batches),
			"limit", w.config.MaxStoredBatches,
			"ownUncommitted", skippedOwn,
			"workerID", w.config.ID)
	}
}

// HasBatch returns true if the worker has the batch with the given digest.
// Does not update LRU (read-only check).
func (w *Worker) HasBatch(digest types.BatchDigest) bool {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	_, ok := w.batches[digest]
	return ok
}

// BatchDigests returns the digests of all stored batches.
// Does not update LRU (read-only access).
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
