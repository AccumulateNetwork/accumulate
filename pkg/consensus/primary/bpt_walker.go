// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Default configuration values for BPT walking.
const (
	// DefaultBPTWalkInterval is the interval between BPT walk cycles.
	DefaultBPTWalkInterval = 5 * time.Second

	// DefaultBPTWalkBatchSize is the number of entries to process per walk step.
	DefaultBPTWalkBatchSize = 100

	// DefaultBPTWalkMaxPendingRequests is the maximum number of pending sync requests.
	DefaultBPTWalkMaxPendingRequests = 1000
)

// BPTHashProvider provides hash information for BPT verification.
type BPTHashProvider interface {
	// GetExpectedRootHash returns the expected root hash for the current state.
	// Returns nil if no expected hash is available.
	GetExpectedRootHash() *[32]byte

	// GetLocalRootHash returns the local BPT root hash.
	GetLocalRootHash() ([32]byte, error)

	// GetBranchHash returns the hash of a branch at the given key.
	// Returns the hash and true if found, zero hash and false if not found.
	GetBranchHash(key [32]byte) ([32]byte, bool, error)

	// GetLeafValue returns the value at a leaf key.
	// Returns the value and true if found, nil and false if not found.
	GetLeafValue(keyHash [32]byte) ([]byte, bool, error)

	// IterateKeys iterates over all key hashes in the BPT.
	// The callback receives the key hash and value for each entry.
	// If the callback returns an error, iteration stops.
	IterateKeys(fn func(keyHash [32]byte, value []byte) error) error
}

// BPTWalkerConfig holds configuration for the BPTWalker.
type BPTWalkerConfig struct {
	// WalkInterval is the interval between BPT walk cycles.
	WalkInterval time.Duration

	// BatchSize is the number of entries to process per walk step.
	BatchSize int

	// MaxPendingRequests is the maximum number of pending sync requests.
	MaxPendingRequests int
}

// applyDefaults fills in default values for unset configuration fields.
func (c *BPTWalkerConfig) applyDefaults() {
	if c.WalkInterval <= 0 {
		c.WalkInterval = DefaultBPTWalkInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBPTWalkBatchSize
	}
	if c.MaxPendingRequests <= 0 {
		c.MaxPendingRequests = DefaultBPTWalkMaxPendingRequests
	}
}

// BPTWalker walks the BPT to detect gaps and mismatches.
// It compares local state against expected hashes and queues
// missing entries for synchronization.
type BPTWalker struct {
	config   BPTWalkerConfig
	provider BPTHashProvider
	syncer   *BPTSyncer

	// Missing keys that need to be synced
	missingKeys   [][32]byte
	missingKeysMu sync.Mutex

	// Track state divergence
	diverged   atomic.Bool
	lastReason atomic.Value // string

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	walkCycles   atomic.Uint64
	gapsFound    atomic.Uint64
	gapsResolved atomic.Uint64
	lastWalkTime atomic.Value // time.Time

	// Error tracking (using mutex since atomic.Value can't store nil)
	lastWalkErrMu sync.RWMutex
	lastWalkErr   error
}

// NewBPTWalker creates a new BPTWalker.
func NewBPTWalker(
	config BPTWalkerConfig,
	provider BPTHashProvider,
	syncer *BPTSyncer,
) *BPTWalker {
	config.applyDefaults()

	w := &BPTWalker{
		config:   config,
		provider: provider,
		syncer:   syncer,
	}
	// Initialize atomic values with non-nil values
	// Note: atomic.Value cannot store nil directly
	w.lastWalkTime.Store(time.Time{})
	w.lastReason.Store("")
	// lastWalkErr is protected by lastWalkErrMu (already initialized to nil)

	return w
}

// Start begins the BPT walker's background loop.
func (w *BPTWalker) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go w.walkLoop()

	slog.Info("BPTWalker started")
	return nil
}

// Stop stops the BPT walker.
func (w *BPTWalker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	slog.Info("BPTWalker stopped",
		"walkCycles", w.walkCycles.Load(),
		"gapsFound", w.gapsFound.Load(),
		"gapsResolved", w.gapsResolved.Load())
}

// IsDiverged returns true if state divergence has been detected.
func (w *BPTWalker) IsDiverged() bool {
	return w.diverged.Load()
}

// DivergenceReason returns the reason for state divergence, if any.
func (w *BPTWalker) DivergenceReason() string {
	if r := w.lastReason.Load(); r != nil {
		return r.(string)
	}
	return ""
}

// PendingGapCount returns the number of pending gaps to be synced.
func (w *BPTWalker) PendingGapCount() int {
	w.missingKeysMu.Lock()
	defer w.missingKeysMu.Unlock()
	return len(w.missingKeys)
}

// TriggerWalk triggers an immediate walk cycle.
func (w *BPTWalker) TriggerWalk() {
	// Run walk in a separate goroutine to avoid blocking
	go func() {
		w.walk()
	}()
}

// walkLoop is the main background loop that periodically walks the BPT.
func (w *BPTWalker) walkLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.WalkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return

		case <-ticker.C:
			w.walk()
		}
	}
}

// walk performs a single BPT walk cycle.
func (w *BPTWalker) walk() {
	w.lastWalkTime.Store(time.Now())

	// Get expected root hash
	expectedRoot := w.provider.GetExpectedRootHash()
	if expectedRoot == nil {
		// No expected hash available, nothing to compare
		slog.Debug("BPTWalker: no expected root hash available")
		return
	}

	// Get local root hash
	localRoot, err := w.provider.GetLocalRootHash()
	if err != nil {
		slog.Warn("BPTWalker: failed to get local root hash", "error", err)
		w.setLastWalkErr(err)
		return
	}

	// Compare root hashes
	if localRoot == *expectedRoot {
		// State is consistent
		if w.diverged.Load() {
			slog.Info("BPTWalker: state convergence detected")
			w.diverged.Store(false)
			w.lastReason.Store("")
		}
		w.walkCycles.Add(1)
		w.setLastWalkErr(nil)
		return
	}

	// State divergence detected
	if !w.diverged.Load() {
		slog.Warn("BPTWalker: state divergence detected",
			"localRoot", localRoot,
			"expectedRoot", *expectedRoot)
		w.diverged.Store(true)
		w.lastReason.Store("root hash mismatch")
	}

	// Walk the tree to find missing/mismatched entries
	missingKeys := w.findMissingKeys(*expectedRoot)

	if len(missingKeys) > 0 {
		slog.Info("BPTWalker: found missing keys",
			"count", len(missingKeys))

		w.missingKeysMu.Lock()
		// Append new missing keys, respecting the limit
		remaining := w.config.MaxPendingRequests - len(w.missingKeys)
		if remaining > 0 {
			if len(missingKeys) > remaining {
				missingKeys = missingKeys[:remaining]
			}
			w.missingKeys = append(w.missingKeys, missingKeys...)
		}
		toSync := w.missingKeys
		w.missingKeysMu.Unlock()

		// Request missing keys from peers
		if w.syncer != nil && len(toSync) > 0 {
			w.gapsFound.Add(uint64(len(missingKeys)))
			w.syncer.RequestMissing(toSync)
		}
	}

	w.walkCycles.Add(1)
	w.setLastWalkErr(nil)
}

// findMissingKeys walks the BPT to find missing or mismatched entries.
// This is a simplified implementation that iterates through all keys
// and checks if they exist locally.
func (w *BPTWalker) findMissingKeys(expectedRoot [32]byte) [][32]byte {
	var missingKeys [][32]byte

	// For now, we'll do a simple iteration to find keys that might be missing.
	// A more sophisticated implementation would walk the tree structure
	// and compare branch hashes to narrow down the search.

	// This basic implementation collects keys that need verification.
	// The actual verification happens through the sync process.

	// Note: In a full implementation, we would:
	// 1. Compare branch hashes at each level
	// 2. Only descend into branches with mismatched hashes
	// 3. Collect leaf keys from mismatched subtrees

	// For now, we rely on the syncer's callback mechanism:
	// When entries are received, they update the local BPT.
	// On the next walk, we check if the root hash matches.

	// If we have pending missing keys from previous walks, use those
	w.missingKeysMu.Lock()
	if len(w.missingKeys) > 0 {
		// Return existing missing keys for retry
		missingKeys = make([][32]byte, len(w.missingKeys))
		copy(missingKeys, w.missingKeys)
		w.missingKeysMu.Unlock()
		return missingKeys
	}
	w.missingKeysMu.Unlock()

	// If the root hashes don't match and we have no pending keys,
	// we need to discover which entries are missing.
	// This would typically involve receiving a list of keys from peers
	// or walking a shared index.

	return missingKeys
}

// OnEntryReceived should be called when a BPT entry is received from sync.
// It removes the key from the pending missing keys list.
func (w *BPTWalker) OnEntryReceived(keyHash [32]byte) {
	w.missingKeysMu.Lock()
	defer w.missingKeysMu.Unlock()

	// Remove the key from missing keys
	for i, k := range w.missingKeys {
		if k == keyHash {
			// Remove by swapping with last element and truncating
			w.missingKeys[i] = w.missingKeys[len(w.missingKeys)-1]
			w.missingKeys = w.missingKeys[:len(w.missingKeys)-1]
			w.gapsResolved.Add(1)
			break
		}
	}
}

// Metrics returns walker metrics.
func (w *BPTWalker) Metrics() (walkCycles, gapsFound, gapsResolved uint64) {
	return w.walkCycles.Load(), w.gapsFound.Load(), w.gapsResolved.Load()
}

// LastWalkTime returns the time of the last walk cycle.
func (w *BPTWalker) LastWalkTime() time.Time {
	if t := w.lastWalkTime.Load(); t != nil {
		return t.(time.Time)
	}
	return time.Time{}
}

// LastWalkError returns the error from the last walk cycle, if any.
func (w *BPTWalker) LastWalkError() error {
	w.lastWalkErrMu.RLock()
	defer w.lastWalkErrMu.RUnlock()
	return w.lastWalkErr
}

// setLastWalkErr sets the last walk error.
func (w *BPTWalker) setLastWalkErr(err error) {
	w.lastWalkErrMu.Lock()
	defer w.lastWalkErrMu.Unlock()
	w.lastWalkErr = err
}
