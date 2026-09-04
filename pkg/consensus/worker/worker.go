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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default configuration values.
const (
	DefaultBatchSize = 500 // max transactions per batch
	// DefaultBatchTimeout is the latency floor for a quiet worker: how long
	// a lone transaction waits before it is sealed without company. It is
	// not the seal trigger under load -- that is BatchSize -- and at 100 ms
	// it was: sixteen workers at 250 tps sealed one or two transactions a
	// batch, ~160 batches a second, and every count downstream held seconds
	// of traffic (C1, #4206). One second is the block interval.
	DefaultBatchTimeout    = time.Second
	DefaultMaxBatchBytes   = 500 * 1024       // 500KB max batch size
	DefaultMaxPendingSize  = 10 * 1024 * 1024 // 10MB max pending transactions
	DefaultMaxPendingCount = 10000            // max pending transaction count
	// Byte caps. Batch COUNT caps do not bound memory: at 700 tx/s the
	// gossip store filled to its count cap holding 728MB of batch bytes per
	// node instance — two instances per 4GiB cgroup — and the fleet was
	// OOM-killed (#4164, runs 20260824T065208Z and 20260824T112437Z). The
	// governor has to be measured in bytes.
	//
	// These are PER-PARTITION budgets. consensus.NewNode divides them among
	// the workers before constructing each one (see perWorkerBytes), so
	// raising num-workers scales parallelism without scaling memory. Read as
	// per-worker they were multiplied by the worker count: 4 workers meant
	// 256MB per partition and 512MB on a dual validator.
	DefaultMaxStoredBatchBytes   = 32 << 20 // active store: 32MB per partition
	DefaultMaxRetainedBatchBytes = 32 << 20 // retention window: 32MB per partition
	DefaultMaxBatchQueueSize     = 1000     // max batches in available queue before blocking
)

// ErrWorkerClosed is returned when operations are attempted on a closed worker.
var ErrWorkerClosed = errors.New("worker is closed")

// ErrBackpressure was returned when the worker refused a transaction because a
// limit had been reached.  Submit no longer refuses: crossing a pending
// boundary SEALS instead, because refusing there discards work at the exact
// moment the fix is to turn it into a batch (#4165).  Kept for callers that
// still test for it.
var ErrBackpressure = errors.New("worker backpressure: pending transactions exceed limit")

// ErrTransactionTooLarge is returned when a single transaction exceeds the
// batch byte limit. A distinct error, not backpressure: backpressure clears
// on retry, this never will — the transaction cannot fit in any batch, and
// accepting it quietly distorted batching instead of refusing it (#4141).
var ErrTransactionTooLarge = errors.New("transaction exceeds the batch size limit")

// ErrValidationFailed is returned when a transaction fails pre-batch validation.
var ErrValidationFailed = errors.New("transaction validation failed")

// ErrStoreFull is returned by SubmitUser when this worker's own uncommitted
// batches and pending transactions fill its byte share. Own batches cannot
// be evicted -- the worker is responsible for them reaching a certificate --
// so the bound is refusing work, not growing (consensus spec, invariant 4).
// The API returns it as NotReady: retry later. Internal traffic (Submit) is
// never refused; it is what drains the store (#4165).
var ErrStoreFull = errors.New("worker store full: own uncommitted batches fill the budget")

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

	// MaxPendingSize is the size of pending transactions at which the worker
	// seals a batch.  Crossing it keeps the envelope and triggers the seal;
	// the queue is bounded by the batch it causes, not by a rejection.
	// Defaults to DefaultMaxPendingSize.
	MaxPendingSize int

	// MaxPendingCount is the number of pending transactions at which the
	// worker seals a batch.  Crossing it keeps the envelope and triggers the
	// seal.  Defaults to DefaultMaxPendingCount.
	MaxPendingCount int

	// MaxStoredBatchBytes bounds the active store in BYTES; the count cap
	// alone let 728MB of batches accumulate (#4164). Defaults to
	// DefaultMaxStoredBatchBytes.
	MaxStoredBatchBytes int

	// MaxRetainedBatchBytes bounds the retention window in bytes. Defaults
	// to DefaultMaxRetainedBatchBytes.
	MaxRetainedBatchBytes int

	// MaxStoredBatches is the maximum number of batches to store.
	// When exceeded, random batches are evicted to make room.
	// This prevents unbounded memory growth from gossip batches.
	// Zero, the default, means no count limit: the store is bounded in bytes
	// (consensus spec, invariant 1). A count is only for tests.
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
	// Defaults to DefaultRetainCommittedFor; a zero MaxRetainedBatches means
	// no count limit, only bytes, and a negative one turns retention off.
	// Negative disables retention, restoring the old delete-on-commit
	// behaviour — which strands any node that misses the commit (#4128).
	RetainCommittedFor time.Duration
	MaxRetainedBatches int

	// Certified reports whether a certified header names a batch. A batch it
	// reports is never re-proposed (consensus spec, invariant 7): the DAG has
	// it, and the executor will retire it when that certificate's block is
	// produced. Nil means "unknown", and re-proposal falls back to age alone.
	Certified func(types.BatchDigest) bool
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
	if c.MaxStoredBatchBytes <= 0 {
		c.MaxStoredBatchBytes = DefaultMaxStoredBatchBytes
	}
	if c.MaxRetainedBatchBytes <= 0 {
		c.MaxRetainedBatchBytes = DefaultMaxRetainedBatchBytes
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

// batchBytes approximates a batch's memory footprint: transaction payloads
// plus per-slice overhead. Exactness does not matter; monotonicity does.
func batchBytes(b *types.Batch) int {
	if b == nil {
		return 0
	}
	n := 128
	for _, tx := range b.Transactions {
		n += len(tx) + 24
	}
	return n
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

	// pins counts, per batch, how many headers await this node's vote while
	// naming it.  A pinned batch is not evictable (#4165 soak
	// 20260902T231641Z).
	//
	// `own` was the only protection before, and it protects the wrong half of
	// the problem.  A header names several batches; if one is missing the vote
	// defers and the others -- which this node DOES hold and will need the
	// moment the missing one lands -- stay evictable.  Under load they were
	// evicted while the node waited, so the next rebroadcast found a
	// DIFFERENT batch missing and deferred again: 777 fetches for 172 distinct
	// batches, one asked 29 times, while the store turned over 1.7 times a
	// second and BVN2 stopped producing blocks.
	//
	// The store's assumption was that a gossiped batch is "just cache" because
	// the author still has it.  That is true of the DATA and false of the
	// TIME: re-fetching is only free if the re-fetch wins the race against the
	// next eviction, and at 17 evictions a second it does not.  A batch some
	// header is waiting on is not cache, it is a prerequisite for progress.
	pins map[types.BatchDigest]int

	// Tombstones: why a batch left the store. A committed certificate whose
	// batch is absent everywhere halts the partition permanently (#4125), and
	// without this the log cannot say whether it was pruned, evicted, or never
	// held. Bounded by maxTombstones. Guarded by batchMu.
	gone map[types.BatchDigest]BatchGone

	// ownBytes is the byte size of own uncommitted batches in the store; with
	// pendingSize it is what SubmitUser refuses against. Under batchMu.
	ownBytes int
	// overLimit is the full-store state, logged on transition and counted
	// while it holds (invariant 5). overLimitChanges counts transitions.
	overLimit        bool
	overLimitChanges atomic.Uint64
	lastEvictLog     time.Time
	goneOrder        []types.BatchDigest
	maxTombstones    int

	// Committed batches kept fetchable for peers that fell behind (#4128).
	// Separate from `batches` on purpose: `batches` is what this node still
	// owes consensus, `retained` is what it can still serve. Guarded by
	// batchMu.
	retained      map[types.BatchDigest]*retainedBatch
	retainedOrder []types.BatchDigest
	maxRetained   int
	retainFor     time.Duration
	// storedBytes/retainedBytes track the byte size of the active and
	// retention stores; guarded by batchMu.
	storedBytes    int
	retainedBytes  int
	maxStoredBytes int
	// maxOwnBytes bounds own uncommitted batches plus pending; SubmitUser
	// refuses against it. It is separate from maxStoredBytes, which bounds
	// the peer cache by eviction: a full own store must not empty the cache
	// of what the next header's vote needs (consensus spec, invariant 8).
	maxOwnBytes int
	// refusing is the state SubmitUser is in, logged on transition.
	refusing         bool
	refusingChanges  atomic.Uint64
	maxRetainedBytes int

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
		pins:                make(map[types.BatchDigest]int),
		lruList:             list.New(),
		gone:                make(map[types.BatchDigest]BatchGone),
		maxTombstones:       config.MaxTombstones,
		retained:            make(map[types.BatchDigest]*retainedBatch),
		maxRetained:         config.MaxRetainedBatches,
		retainFor:           config.RetainCommittedFor,
		maxStoredBytes:      config.MaxStoredBatchBytes,
		maxOwnBytes:         config.MaxStoredBatchBytes,
		maxRetainedBytes:    config.MaxRetainedBatchBytes,
		availableBatchQueue: make(chan types.BatchDigest, config.MaxBatchQueueSize),
		triggerBatch:        make(chan struct{}, 1),
		triggerEviction:     make(chan struct{}, 1),
	}
}

// Submit adds a transaction to the pending batch.  It does not refuse work:
// crossing a pending boundary seals a batch rather than rejecting the envelope
// that crossed it.
// Returns ErrWorkerClosed if the worker has been closed.
// Returns ErrValidationFailed (wrapped) if the transaction fails validation.
// Submit accepts a transaction from an internal source -- a synthetic, an
// anchor, a healer's re-submission -- and never refuses it for lack of room:
// that traffic is what drains the store (#4165).
func (w *Worker) Submit(tx []byte) error { return w.submit(tx, false) }

// SubmitUser accepts a user's transaction from the API, and refuses with
// ErrStoreFull while this worker's own uncommitted batches and pending
// transactions fill its byte share (consensus spec, invariant 4).
func (w *Worker) SubmitUser(tx []byte) error { return w.submit(tx, true) }

func (w *Worker) submit(tx []byte, bounded bool) error {
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

	// The batch store being full is not a reason to refuse a transaction.
	// Accepting one does not grow the store -- only SEALING does -- and the
	// store is bounded by eviction, which now leaves alone the batches a vote
	// is waiting on.  Refusing here rejected cross-partition synthetics and
	// the healer's own re-submissions, so the stream that needed the store to
	// drain was the stream being turned away (#4165).

	if txTraceEnabled {
		slog.Info("TX accepted", "tx", txID(tx), "worker", w.config.ID,
			"partition", w.config.Partition, "bytes", len(tx))
	}

	w.batchMu.Lock()
	own := w.ownBytes
	w.batchMu.Unlock()

	w.mu.Lock()

	// A user's transaction must fit beside own uncommitted batches and what
	// is pending. When it does not, the answer is "not now", and the store
	// stays within its budget however far commits lag.
	if bounded && own+w.pendingSize+len(tx) > w.maxOwnBytes {
		pending := w.pendingSize
		w.mu.Unlock()
		w.txnsRejected.Add(1)
		w.setRefusing(true, own, pending)
		return fmt.Errorf("%w: own %d + pending %d bytes of %d", ErrStoreFull, own, pending, w.maxOwnBytes)
	}
	if bounded && w.refusing {
		w.setRefusing(false, own, w.pendingSize)
	}

	// Copy the transaction to avoid external modification
	txCopy := make([]byte, len(tx))
	copy(txCopy, tx)

	w.pending = append(w.pending, txCopy)
	w.pendingSize += len(txCopy)

	// The ordinary batching trigger: a batch is worth making.  Signalled to
	// the batch loop, and it does not matter if a signal is coalesced.
	shouldTrigger := len(w.pending) >= w.config.BatchSize ||
		w.pendingSize >= w.config.MaxBatchBytes

	// The BOUNDARY: pending is as large as it is allowed to get.  Crossing it
	// keeps the envelope and seals, rather than refusing the transaction --
	// refusing discards work at the exact moment the fix is to turn it into a
	// batch, and the queue then stays full because nothing sealed it.
	//
	// This seal is SYNCHRONOUS, and that is what bounds pending.  Signalling
	// the batch loop instead is a non-blocking send that coalesces: under load
	// the trigger is already queued, the signal is dropped, and pending grows
	// without limit.  Sealing here means the queue can never be more than the
	// one envelope that crossed the boundary.
	mustSeal := len(w.pending) >= w.config.MaxPendingCount ||
		w.pendingSize >= w.config.MaxPendingSize

	w.mu.Unlock()

	if mustSeal && w.ctx != nil {
		// Synchronous, and it may block: the available-batch queue applies
		// backpressure when consensus is not consuming batches fast enough.
		// Blocking the submitter is the point -- it slows the source instead
		// of dropping its work, which is what refusing did.
		w.createAndBroadcastBatch()
		return nil
	}

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
	if w.overStoreLimit() {
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
	w.storedBytes += batchBytes(batch)
	w.observeStore()

	return nil
}

// overStoreLimit reports whether the active store exceeds its byte budget,
// or its count limit if a test set one. The caller must hold batchMu.
func (w *Worker) overStoreLimit() bool {
	return w.peerBytes() > w.maxStoredBytes ||
		(w.config.MaxStoredBatches > 0 && len(w.batches) > w.config.MaxStoredBatches)
}

// peerBytes is what the peer cache holds: the store less own batches, which
// have their own budget (invariant 8). The caller must hold batchMu.
func (w *Worker) peerBytes() int { return w.storedBytes - w.ownBytes }

// setRefusing records whether SubmitUser is refusing, logging the transition
// and counting it (invariant 5).
func (w *Worker) setRefusing(on bool, own, pending int) {
	w.batchMu.Lock()
	defer w.batchMu.Unlock()
	if on == w.refusing {
		return
	}
	w.refusing = on
	w.refusingChanges.Add(1)
	id := strconv.Itoa(int(w.config.ID))
	if on {
		metrics.BatchStoreRefusing.WithLabelValues(w.config.Partition, id).Set(1)
		slog.Warn("Refusing user submissions: own uncommitted batches fill the share (commit is lagging)",
			"ownBytes", own, "pendingBytes", pending, "shareBytes", w.maxOwnBytes,
			"workerID", w.config.ID, "partition", w.config.Partition)
	} else {
		metrics.BatchStoreRefusing.WithLabelValues(w.config.Partition, id).Set(0)
		slog.Info("Accepting user submissions again",
			"ownBytes", own, "pendingBytes", pending, "shareBytes", w.maxOwnBytes,
			"workerID", w.config.ID, "partition", w.config.Partition)
	}
}

// observeStore publishes the store's own and peer bytes. The caller must
// hold batchMu.
func (w *Worker) observeStore() {
	id := strconv.Itoa(int(w.config.ID))
	metrics.BatchStoreBytes.WithLabelValues(w.config.Partition, id, "own").Set(float64(w.ownBytes))
	metrics.BatchStoreBytes.WithLabelValues(w.config.Partition, id, "peer").Set(float64(w.storedBytes - w.ownBytes))
}

// storeOwn puts a batch this worker sealed into the store. Own batches are
// never evicted; their bytes are what SubmitUser refuses against.
func (w *Worker) storeOwn(batch *types.Batch) {
	digest := batch.Digest()
	w.batchMu.Lock()
	if w.overStoreLimit() {
		select {
		case w.triggerEviction <- struct{}{}:
		default:
		}
	}
	element := w.lruList.PushFront(digest)
	w.batches[digest] = &lruEntry{
		batch:      batch,
		element:    element,
		own:        true,
		lastQueued: time.Now(),
	}
	w.storedBytes += batchBytes(batch)
	w.ownBytes += batchBytes(batch)
	w.observeStore()
	w.batchMu.Unlock()
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
// ConsumeAvailableBatches drains the availability queue for a header. A digest
// can wait in the queue for as long as headers are slow to take it, and by
// then its batch may have been certified through an earlier re-queue and
// retired by execution. Proposing it again puts one batch in two certificates
// and the second cannot be served (run 20260904T012004Z: a header at round
// 4524 named a batch retired 57 minutes earlier). So what is returned is
// filtered to what is proposable: in the active store, and named by no
// certified header (consensus spec, invariant 7).
func (w *Worker) ConsumeAvailableBatches() []types.BatchDigest {
	return w.proposable(w.drainAvailable())
}

func (w *Worker) drainAvailable() []types.BatchDigest {
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
			w.storedBytes -= batchBytes(entry.batch)
			if entry.own {
				w.ownBytes -= batchBytes(entry.batch)
			}
			// Committed, so it leaves the active store and stops being
			// re-proposed — but keep it fetchable for a while, because a peer
			// that missed this commit has nowhere else to get it (#4128).
			w.retain(digest, entry.batch, info.Detail, info.Cert)
			w.noteGone(digest, GonePruned, info.Detail, info.Cert)
			pruned++
		}
	}

	w.observeStore()
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
	w.storeOwn(batch)

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

		stale, staleBatches := w.staleOwnBatches(time.Now())

		// Re-BROADCAST the batch bytes, not just the digest (#4159). A batch
		// is broadcast exactly once at creation; if that publish was lost
		// (forming mesh, receiver channel full), NOTHING else ever re-sent
		// the bytes — headers rebroadcast, votes resend, certificates sync,
		// but batches had no retry. A batch still here after ReproposeAfter
		// is a batch the network did not commit, and "peers never got it" is
		// one of the two reasons why; re-sending the bytes costs one publish
		// and un-poisons every header that names the digest. Voters defer
		// their vote until they HOLD the batch, so a lost batch otherwise
		// blocks its author's headers from quorum forever.
		if w.gossip != nil {
			for _, b := range staleBatches {
				if err := w.gossip.BroadcastBatch(w.ctx, b); err != nil {
					slog.Debug("Re-broadcast of stale batch failed",
						"error", err, "worker", w.config.ID)
				}
			}
		}

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

	if !w.overStoreLimit() {
		w.setOverLimit(false, 0)
		return // No eviction needed unless we exceed a limit
	}

	// Evict down to 90% of the byte cap, which is what bounds memory
	// (#4164, consensus spec invariant 1), and of the count cap if a test
	// set one. Evicting to 90% rather than to the cap keeps the store from
	// parking permanently above its own limit.
	targetCount := 0 // no count limit unless a test set one
	if c := w.config.MaxStoredBatches; c > 0 {
		targetCount = max(1, c*9/10)
	}
	targetBytes := int(float64(w.maxStoredBytes) * 0.9)
	overTarget := func() bool {
		return w.peerBytes() > targetBytes || (targetCount > 0 && len(w.batches) > targetCount)
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
	evicted, skippedOwn, skippedPinned := 0, 0, 0
	for e := w.lruList.Back(); e != nil && overTarget(); {
		prev := e.Prev()
		lruDigest := e.Value.(types.BatchDigest)
		entry, ok := w.batches[lruDigest]
		if ok && entry.own {
			skippedOwn++
			e = prev
			continue
		}
		if w.pins[lruDigest] > 0 {
			// A header is waiting on this to vote.  Evicting it is not
			// reclaiming cache, it is undoing work that has to be redone
			// before the partition can advance.
			skippedPinned++
			e = prev
			continue
		}
		w.lruList.Remove(e)
		delete(w.batches, lruDigest)
		if ok {
			w.storedBytes -= batchBytes(entry.batch)
		}
		w.noteGone(lruDigest, GoneEvicted,
			fmt.Sprintf("store over limit (%d batches / %d bytes)", w.config.MaxStoredBatches, w.maxStoredBytes), "")
		evicted++
		e = prev
	}

	w.observeStore()
	if evicted > 0 {
		// A summary at most once a second per worker; the rest is Debug.
		// Run 20260903T173742Z logged 208,150 of these in nine minutes.
		level := slog.LevelDebug
		if time.Since(w.lastEvictLog) >= time.Second {
			level, w.lastEvictLog = slog.LevelWarn, time.Now()
		}
		slog.Log(context.Background(), level, "Evicted batches due to storage limit (LRU)",
			"evicted", evicted,
			"remaining", len(w.batches),
			"skippedOwnUncommitted", skippedOwn,
			"skippedPinned", skippedPinned,
			"workerID", w.config.ID)
	}
	// The peer cache could not reach its target because what is left is
	// pinned: headers are waiting on more than the budget holds. A state,
	// logged when it changes and counted while it holds (invariant 5). Own
	// batches have their own budget and are SubmitUser's to refuse on.
	w.setOverLimit(overTarget() && skippedPinned > 0, skippedPinned)
}

// setOverLimit records whether the peer cache is over its budget with only
// pinned batches left to evict, logging the transition and counting it. The
// caller must hold batchMu.
func (w *Worker) setOverLimit(over bool, pinned int) {
	if over == w.overLimit {
		return
	}
	w.overLimit = over
	w.overLimitChanges.Add(1)
	if over {
		slog.Warn("Peer batch cache over budget with only pinned batches left (headers are waiting on more than it holds)",
			"stored", len(w.batches),
			"peerBytes", w.peerBytes(),
			"ownBytes", w.ownBytes,
			"budgetBytes", w.maxStoredBytes,
			"pinned", pinned,
			"workerID", w.config.ID)
	} else {
		slog.Info("Peer batch cache back within budget",
			"stored", len(w.batches),
			"peerBytes", w.peerBytes(),
			"budgetBytes", w.maxStoredBytes,
			"workerID", w.config.ID)
	}
}

// PinBatches marks batches as required by a header awaiting this node's vote,
// so LRU eviction leaves them alone.  Calls nest: a batch named by two pending
// headers is pinned twice and released twice.
//
// Pinning does NOT fetch.  A pin on a batch this node does not hold is still
// meaningful -- it says "when this arrives, keep it" -- and costs one map entry.
func (w *Worker) PinBatches(digests []types.BatchDigest) {
	if len(digests) == 0 {
		return
	}
	w.batchMu.Lock()
	defer w.batchMu.Unlock()
	if w.pins == nil {
		w.pins = map[types.BatchDigest]int{}
	}
	for _, d := range digests {
		w.pins[d]++
	}
}

// UnpinBatches releases pins taken by PinBatches: the vote went out, or the
// header was abandoned.
//
// Releasing is what keeps the pin set bounded, and it must happen on BOTH
// paths.  A pin that is never released is a batch that can never be evicted,
// which is the memory bound failing open rather than closed.
func (w *Worker) UnpinBatches(digests []types.BatchDigest) {
	if len(digests) == 0 {
		return
	}
	w.batchMu.Lock()
	defer w.batchMu.Unlock()
	for _, d := range digests {
		if n := w.pins[d]; n <= 1 {
			delete(w.pins, d)
		} else {
			w.pins[d] = n - 1
		}
	}
}

// PinnedBatches reports how many batches are currently pinned. For tests and
// diagnostics.
func (w *Worker) PinnedBatches() int {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	return len(w.pins)
}

// HasBatch returns true if the worker has the batch with the given digest.
// Does not update LRU (read-only check).
func (w *Worker) HasBatch(digest types.BatchDigest) bool {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	_, ok := w.batches[digest]
	return ok
}

// CanServeBatch reports whether this worker HOLDS the batch in any form —
// active store or retained (committed-and-kept-fetchable). The vote-time
// availability gate (#4159) asks THIS question, not HasBatch: a validator
// that already executed a batch obviously has it, and answering no made it
// refuse to vote for any header re-proposing that digest — including every
// fresh batch riding in the same header. HasBatch stays active-store-only:
// pruning and re-proposal logic depend on that meaning.
func (w *Worker) CanServeBatch(digest types.BatchDigest) bool {
	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	if _, ok := w.batches[digest]; ok {
		return true
	}
	_, ok := w.retained[digest]
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

// staleOwnBatches returns the own batches that have waited longer than
// ReproposeAfter without a certificate, and stamps them as re-queued. A
// batch a certified header already names is not stale whatever its age: the
// DAG has it, and proposing it again puts one batch in two certificates
// (consensus spec, invariant 7; C5, #4210). The caller re-broadcasts and
// re-queues what is returned.
func (w *Worker) staleOwnBatches(now time.Time) (stale []types.BatchDigest, batches []*types.Batch) {
	w.batchMu.Lock()
	defer w.batchMu.Unlock()
	certified := 0
	for digest, entry := range w.batches {
		if !entry.own || now.Sub(entry.lastQueued) <= w.config.ReproposeAfter {
			continue
		}
		if w.config.Certified != nil && w.config.Certified(digest) {
			certified++
			entry.lastQueued = now // ask again after another ReproposeAfter, not every tick
			continue
		}
		stale = append(stale, digest)
		batches = append(batches, entry.batch)
		entry.lastQueued = now
	}
	if certified > 0 {
		slog.Debug("Own batches already certified, not re-proposed", "count", certified, "worker", w.config.ID)
	}
	return stale, batches
}

// proposable filters digests down to those whose batch is still in the active
// store and that no certified header names.
func (w *Worker) proposable(digests []types.BatchDigest) []types.BatchDigest {
	if len(digests) == 0 {
		return digests
	}
	w.batchMu.Lock()
	defer w.batchMu.Unlock()
	kept := digests[:0]
	dropped := 0
	for _, d := range digests {
		if _, ok := w.batches[d]; !ok {
			dropped++
			continue
		}
		if w.config.Certified != nil && w.config.Certified(d) {
			dropped++
			continue
		}
		kept = append(kept, d)
	}
	if dropped > 0 {
		slog.Debug("Queued batches no longer proposable", "dropped", dropped, "worker", w.config.ID)
	}
	return kept
}
