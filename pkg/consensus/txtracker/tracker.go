// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package txtracker provides transaction status tracking for DAG-BFT consensus.
package txtracker

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// Status represents the current status of a tracked transaction.
type Status int

const (
	// StatusUnknown indicates the transaction is not being tracked.
	StatusUnknown Status = iota

	// StatusPending indicates the transaction has been submitted but not yet batched.
	StatusPending

	// StatusBatched indicates the transaction is included in a batch.
	StatusBatched

	// StatusCertified indicates the batch containing this transaction has been certified.
	StatusCertified

	// StatusCommitted indicates the transaction has been committed to a block.
	StatusCommitted

	// StatusFailed indicates the transaction processing failed.
	StatusFailed

	// StatusExpired indicates the transaction tracking expired without completion.
	StatusExpired
)

// String returns a string representation of the status.
func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "unknown"
	case StatusPending:
		return "pending"
	case StatusBatched:
		return "batched"
	case StatusCertified:
		return "certified"
	case StatusCommitted:
		return "committed"
	case StatusFailed:
		return "failed"
	case StatusExpired:
		return "expired"
	default:
		return "invalid"
	}
}

// ErrTrackerClosed is returned when operations are attempted on a closed tracker.
var ErrTrackerClosed = errors.New("tracker is closed")

// ErrTransactionNotFound is returned when a transaction is not being tracked.
var ErrTransactionNotFound = errors.New("transaction not found")

// ErrTimeout is returned when waiting for a status times out.
var ErrTimeout = errors.New("timeout waiting for transaction status")

// Default configuration values.
const (
	DefaultMaxTracked      = 100000           // Maximum tracked transactions
	DefaultExpiryDuration  = 5 * time.Minute  // Transaction tracking expiry
	DefaultCleanupInterval = 30 * time.Second // Cleanup interval
)

// TrackerConfig holds configuration for Tracker.
type TrackerConfig struct {
	// MaxTracked is the maximum number of transactions to track.
	// Oldest transactions are evicted when exceeded.
	// Defaults to DefaultMaxTracked.
	MaxTracked int

	// ExpiryDuration is how long to track transactions before expiry.
	// Defaults to DefaultExpiryDuration.
	ExpiryDuration time.Duration

	// CleanupInterval is how often to run cleanup of expired transactions.
	// Defaults to DefaultCleanupInterval.
	CleanupInterval time.Duration
}

// applyDefaults fills in default values for unset configuration fields.
func (c *TrackerConfig) applyDefaults() {
	if c.MaxTracked <= 0 {
		c.MaxTracked = DefaultMaxTracked
	}
	if c.ExpiryDuration <= 0 {
		c.ExpiryDuration = DefaultExpiryDuration
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = DefaultCleanupInterval
	}
}

// TxInfo contains information about a tracked transaction.
type TxInfo struct {
	Hash       [32]byte
	Status     Status
	SubmitTime time.Time
	BatchTime  time.Time
	CommitTime time.Time
	BlockIndex uint64
	Error      error
}

// trackedTx is internal tracking state for a transaction.
type trackedTx struct {
	info    TxInfo
	waiters []chan Status
	mu      sync.Mutex
}

// Tracker tracks transaction status through the consensus pipeline.
type Tracker struct {
	config TrackerConfig

	mu    sync.RWMutex
	txns  map[[32]byte]*trackedTx
	order [][32]byte // Track insertion order for LRU eviction

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed bool
}

// NewTracker creates a new transaction tracker.
func NewTracker(config TrackerConfig) *Tracker {
	config.applyDefaults()

	ctx, cancel := context.WithCancel(context.Background())

	t := &Tracker{
		config: config,
		txns:   make(map[[32]byte]*trackedTx),
		order:  make([][32]byte, 0),
		ctx:    ctx,
		cancel: cancel,
	}

	// Start cleanup goroutine
	t.wg.Add(1)
	go t.cleanupLoop()

	return t
}

// Track starts tracking a transaction and returns its hash.
func (t *Tracker) Track(tx []byte) [32]byte {
	hash := sha256.Sum256(tx)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return hash
	}

	// Check if already tracked
	if _, exists := t.txns[hash]; exists {
		return hash
	}

	// Evict oldest if at capacity
	for len(t.txns) >= t.config.MaxTracked && len(t.order) > 0 {
		oldest := t.order[0]
		t.order = t.order[1:]
		if tracked, ok := t.txns[oldest]; ok {
			tracked.mu.Lock()
			// Notify waiters of expiry
			for _, ch := range tracked.waiters {
				select {
				case ch <- StatusExpired:
				default:
				}
				close(ch)
			}
			tracked.waiters = nil
			tracked.mu.Unlock()
			delete(t.txns, oldest)
		}
	}

	// Add new transaction
	t.txns[hash] = &trackedTx{
		info: TxInfo{
			Hash:       hash,
			Status:     StatusPending,
			SubmitTime: time.Now(),
		},
	}
	t.order = append(t.order, hash)

	return hash
}

// TrackWithHash starts tracking a transaction with a known hash.
func (t *Tracker) TrackWithHash(hash [32]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	// Check if already tracked
	if _, exists := t.txns[hash]; exists {
		return
	}

	// Evict oldest if at capacity
	for len(t.txns) >= t.config.MaxTracked && len(t.order) > 0 {
		oldest := t.order[0]
		t.order = t.order[1:]
		delete(t.txns, oldest)
	}

	// Add new transaction
	t.txns[hash] = &trackedTx{
		info: TxInfo{
			Hash:       hash,
			Status:     StatusPending,
			SubmitTime: time.Now(),
		},
	}
	t.order = append(t.order, hash)
}

// SetStatus updates the status of a tracked transaction.
func (t *Tracker) SetStatus(hash [32]byte, status Status) {
	t.mu.RLock()
	tracked, ok := t.txns[hash]
	t.mu.RUnlock()

	if !ok {
		return
	}

	tracked.mu.Lock()
	defer tracked.mu.Unlock()

	// Don't downgrade status (except for failure)
	if tracked.info.Status >= status && status != StatusFailed {
		return
	}

	tracked.info.Status = status

	// Update timestamps
	now := time.Now()
	switch status {
	case StatusBatched:
		tracked.info.BatchTime = now
	case StatusCommitted:
		tracked.info.CommitTime = now
	}

	// Notify waiters
	for _, ch := range tracked.waiters {
		select {
		case ch <- status:
		default:
		}
	}

	// Clear waiters if terminal status
	if status == StatusCommitted || status == StatusFailed || status == StatusExpired {
		for _, ch := range tracked.waiters {
			close(ch)
		}
		tracked.waiters = nil
	}
}

// SetStatusWithBlock updates status with block information.
func (t *Tracker) SetStatusWithBlock(hash [32]byte, status Status, blockIndex uint64) {
	t.mu.RLock()
	tracked, ok := t.txns[hash]
	t.mu.RUnlock()

	if !ok {
		return
	}

	tracked.mu.Lock()
	tracked.info.BlockIndex = blockIndex
	tracked.mu.Unlock()

	t.SetStatus(hash, status)
}

// SetError sets an error for a tracked transaction.
func (t *Tracker) SetError(hash [32]byte, err error) {
	t.mu.RLock()
	tracked, ok := t.txns[hash]
	t.mu.RUnlock()

	if !ok {
		return
	}

	tracked.mu.Lock()
	tracked.info.Error = err
	tracked.mu.Unlock()

	t.SetStatus(hash, StatusFailed)
}

// GetStatus returns the current status of a transaction.
func (t *Tracker) GetStatus(hash [32]byte) (Status, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return StatusUnknown, ErrTrackerClosed
	}

	tracked, ok := t.txns[hash]
	if !ok {
		return StatusUnknown, ErrTransactionNotFound
	}

	tracked.mu.Lock()
	status := tracked.info.Status
	tracked.mu.Unlock()

	return status, nil
}

// GetInfo returns full information about a tracked transaction.
func (t *Tracker) GetInfo(hash [32]byte) (TxInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return TxInfo{}, ErrTrackerClosed
	}

	tracked, ok := t.txns[hash]
	if !ok {
		return TxInfo{}, ErrTransactionNotFound
	}

	tracked.mu.Lock()
	info := tracked.info
	tracked.mu.Unlock()

	return info, nil
}

// WaitForStatus waits until the transaction reaches the specified status.
func (t *Tracker) WaitForStatus(ctx context.Context, hash [32]byte, target Status) error {
	t.mu.RLock()
	tracked, ok := t.txns[hash]
	closed := t.closed
	t.mu.RUnlock()

	if closed {
		return ErrTrackerClosed
	}

	if !ok {
		return ErrTransactionNotFound
	}

	tracked.mu.Lock()
	currentStatus := tracked.info.Status

	// Already at or past target status
	if currentStatus >= target {
		tracked.mu.Unlock()
		if currentStatus == StatusFailed || currentStatus == StatusExpired {
			return errors.New("transaction " + currentStatus.String())
		}
		return nil
	}

	// Register waiter
	ch := make(chan Status, 1)
	tracked.waiters = append(tracked.waiters, ch)
	tracked.mu.Unlock()

	// Wait for status update
	select {
	case <-ctx.Done():
		return ErrTimeout
	case status := <-ch:
		if status >= target {
			if status == StatusFailed || status == StatusExpired {
				return errors.New("transaction " + status.String())
			}
			return nil
		}
		// Status changed but not to target - keep waiting
		return t.WaitForStatus(ctx, hash, target)
	}
}

// Count returns the number of tracked transactions.
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.txns)
}

// Close stops the tracker and releases resources.
func (t *Tracker) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.cancel()
	t.mu.Unlock()

	t.wg.Wait()

	// Clean up all waiters
	t.mu.Lock()
	for _, tracked := range t.txns {
		tracked.mu.Lock()
		for _, ch := range tracked.waiters {
			close(ch)
		}
		tracked.waiters = nil
		tracked.mu.Unlock()
	}
	t.txns = nil
	t.order = nil
	t.mu.Unlock()

	return nil
}

// cleanupLoop periodically removes expired transactions.
func (t *Tracker) cleanupLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.cleanup()
		}
	}
}

// cleanup removes expired transactions.
func (t *Tracker) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	now := time.Now()
	expiry := t.config.ExpiryDuration

	// Build new order slice without expired transactions
	newOrder := make([][32]byte, 0, len(t.order))
	for _, hash := range t.order {
		tracked, ok := t.txns[hash]
		if !ok {
			continue
		}

		tracked.mu.Lock()
		isExpired := now.Sub(tracked.info.SubmitTime) > expiry

		// Don't expire committed transactions (they clean up naturally via LRU)
		if isExpired && tracked.info.Status != StatusCommitted {
			// Notify waiters
			for _, ch := range tracked.waiters {
				select {
				case ch <- StatusExpired:
				default:
				}
				close(ch)
			}
			tracked.waiters = nil
			tracked.info.Status = StatusExpired
			tracked.mu.Unlock()
			delete(t.txns, hash)
		} else {
			tracked.mu.Unlock()
			newOrder = append(newOrder, hash)
		}
	}

	t.order = newOrder
}

// MarkBatchIncluded marks all transactions in a batch as batched.
func (t *Tracker) MarkBatchIncluded(txHashes [][32]byte) {
	for _, hash := range txHashes {
		t.SetStatus(hash, StatusBatched)
	}
}

// MarkCommitted marks all transactions as committed with block info.
func (t *Tracker) MarkCommitted(txHashes [][32]byte, blockIndex uint64) {
	for _, hash := range txHashes {
		t.SetStatusWithBlock(hash, StatusCommitted, blockIndex)
	}
}
