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

// Default configuration values for chain gap filling.
const (
	// DefaultChainGapCheckInterval is the interval between gap checks.
	DefaultChainGapCheckInterval = 5 * time.Second

	// DefaultChainGapBatchSize is the number of entries to request per batch.
	DefaultChainGapBatchSize = 100

	// DefaultChainGapMaxPending is the maximum number of pending gap fills.
	DefaultChainGapMaxPending = 1000
)

// ChainGap represents a gap in a chain that needs to be filled.
type ChainGap struct {
	// ChainKey is the key identifying the chain (typically account + chain name hash).
	ChainKey [32]byte

	// StartIndex is the first missing entry index.
	StartIndex uint64

	// EndIndex is the last missing entry index (exclusive).
	EndIndex uint64

	// ExpectedAnchor is the expected anchor hash after filling the gap.
	ExpectedAnchor [32]byte
}

// ChainGapFillerConfig holds configuration for the ChainGapFiller.
type ChainGapFillerConfig struct {
	// CheckInterval is the interval between gap checks.
	CheckInterval time.Duration

	// BatchSize is the number of entries to request per batch.
	BatchSize int

	// MaxPending is the maximum number of pending gap fills.
	MaxPending int
}

// applyDefaults fills in default values for unset configuration fields.
func (c *ChainGapFillerConfig) applyDefaults() {
	if c.CheckInterval <= 0 {
		c.CheckInterval = DefaultChainGapCheckInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultChainGapBatchSize
	}
	if c.MaxPending <= 0 {
		c.MaxPending = DefaultChainGapMaxPending
	}
}

// ChainEntryProvider provides access to chain data for gap detection and filling.
type ChainEntryProvider interface {
	// GetChainHead returns the current chain head (count and anchor hash).
	// Returns count, anchor hash, and error.
	GetChainHead(chainKey [32]byte) (count uint64, anchor [32]byte, err error)

	// GetExpectedChainAnchor returns the expected anchor for a chain from the BPT.
	// Returns the expected anchor and true if found, zero hash and false if not found.
	GetExpectedChainAnchor(chainKey [32]byte) ([32]byte, bool)

	// IterateChainKeys iterates over all chain keys that need to be checked.
	// The callback receives the chain key and its expected anchor from BPT.
	IterateChainKeys(fn func(chainKey [32]byte, expectedAnchor [32]byte) error) error
}

// ChainEntryRequester requests missing chain entries from peers.
type ChainEntryRequester interface {
	// RequestChainEntries requests chain entries from peers.
	// Returns the entries received (may be partial).
	RequestChainEntries(ctx context.Context, chainKey [32]byte, startIndex, count uint64) ([][]byte, error)

	// InsertChainEntry inserts a chain entry at the given index.
	InsertChainEntry(chainKey [32]byte, index uint64, entry []byte) error
}

// ChainGapFiller fills gaps in chains by requesting missing entries from peers.
type ChainGapFiller struct {
	config    ChainGapFillerConfig
	provider  ChainEntryProvider
	requester ChainEntryRequester

	// Pending gaps
	gaps   []ChainGap
	gapsMu sync.Mutex

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	gapsDetected atomic.Uint64
	gapsFilled   atomic.Uint64
	entriesRecv  atomic.Uint64
	checkCycles  atomic.Uint64
}

// NewChainGapFiller creates a new ChainGapFiller.
func NewChainGapFiller(
	config ChainGapFillerConfig,
	provider ChainEntryProvider,
	requester ChainEntryRequester,
) *ChainGapFiller {
	config.applyDefaults()

	return &ChainGapFiller{
		config:    config,
		provider:  provider,
		requester: requester,
	}
}

// Start begins the chain gap filler's background loop.
func (f *ChainGapFiller) Start(ctx context.Context) error {
	f.ctx, f.cancel = context.WithCancel(ctx)

	f.wg.Add(1)
	go f.gapCheckLoop()

	slog.Info("ChainGapFiller started")
	return nil
}

// Stop stops the chain gap filler.
func (f *ChainGapFiller) Stop() {
	if f.cancel != nil {
		f.cancel()
	}
	f.wg.Wait()
	slog.Info("ChainGapFiller stopped",
		"gapsDetected", f.gapsDetected.Load(),
		"gapsFilled", f.gapsFilled.Load(),
		"entriesRecv", f.entriesRecv.Load())
}

// PendingGapCount returns the number of pending gaps.
func (f *ChainGapFiller) PendingGapCount() int {
	f.gapsMu.Lock()
	defer f.gapsMu.Unlock()
	return len(f.gaps)
}

// AddGap adds a gap to be filled.
func (f *ChainGapFiller) AddGap(gap ChainGap) {
	f.gapsMu.Lock()
	defer f.gapsMu.Unlock()

	// Check if we already have this gap
	for _, g := range f.gaps {
		if g.ChainKey == gap.ChainKey && g.StartIndex == gap.StartIndex {
			return
		}
	}

	// Respect max pending limit
	if len(f.gaps) >= f.config.MaxPending {
		slog.Warn("ChainGapFiller: max pending gaps reached, dropping gap")
		return
	}

	f.gaps = append(f.gaps, gap)
	f.gapsDetected.Add(1)
}

// gapCheckLoop is the main background loop.
func (f *ChainGapFiller) gapCheckLoop() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-ticker.C:
			f.checkAndFillGaps()
		}
	}
}

// checkAndFillGaps checks for gaps and attempts to fill them.
func (f *ChainGapFiller) checkAndFillGaps() {
	f.checkCycles.Add(1)

	// First, detect new gaps
	if f.provider != nil {
		f.detectGaps()
	}

	// Then, attempt to fill existing gaps
	f.fillGaps()
}

// detectGaps scans chains for gaps between local state and expected state.
func (f *ChainGapFiller) detectGaps() {
	if f.provider == nil {
		return
	}

	err := f.provider.IterateChainKeys(func(chainKey [32]byte, expectedAnchor [32]byte) error {
		// Get local chain head
		count, localAnchor, err := f.provider.GetChainHead(chainKey)
		if err != nil {
			// Chain might not exist yet, which is okay
			return nil
		}

		// Compare anchors
		if localAnchor == expectedAnchor {
			// Chain is up to date
			return nil
		}

		// There's a gap - we need more entries
		// We don't know exactly how many, so we request a batch
		gap := ChainGap{
			ChainKey:       chainKey,
			StartIndex:     count,
			EndIndex:       count + uint64(f.config.BatchSize),
			ExpectedAnchor: expectedAnchor,
		}

		f.AddGap(gap)
		return nil
	})

	if err != nil {
		slog.Warn("ChainGapFiller: error detecting gaps", "error", err)
	}
}

// fillGaps attempts to fill pending gaps.
func (f *ChainGapFiller) fillGaps() {
	if f.requester == nil {
		return
	}

	f.gapsMu.Lock()
	gaps := make([]ChainGap, len(f.gaps))
	copy(gaps, f.gaps)
	f.gapsMu.Unlock()

	for _, gap := range gaps {
		if err := f.fillGap(gap); err != nil {
			slog.Debug("ChainGapFiller: failed to fill gap",
				"chainKey", gap.ChainKey,
				"startIndex", gap.StartIndex,
				"error", err)
			continue
		}

		// Remove the gap from pending
		f.removeGap(gap)
		f.gapsFilled.Add(1)
	}
}

// fillGap attempts to fill a single gap.
func (f *ChainGapFiller) fillGap(gap ChainGap) error {
	// Request missing entries
	count := gap.EndIndex - gap.StartIndex
	if count > uint64(f.config.BatchSize) {
		count = uint64(f.config.BatchSize)
	}

	entries, err := f.requester.RequestChainEntries(f.ctx, gap.ChainKey, gap.StartIndex, count)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	// Insert entries
	for i, entry := range entries {
		if err := f.requester.InsertChainEntry(gap.ChainKey, gap.StartIndex+uint64(i), entry); err != nil {
			return err
		}
		f.entriesRecv.Add(1)
	}

	slog.Debug("ChainGapFiller: filled gap",
		"chainKey", gap.ChainKey,
		"startIndex", gap.StartIndex,
		"entriesInserted", len(entries))

	// Check if we need more entries
	if f.provider != nil {
		newCount, newAnchor, err := f.provider.GetChainHead(gap.ChainKey)
		if err == nil && newAnchor != gap.ExpectedAnchor {
			// Still need more entries
			newGap := ChainGap{
				ChainKey:       gap.ChainKey,
				StartIndex:     newCount,
				EndIndex:       newCount + uint64(f.config.BatchSize),
				ExpectedAnchor: gap.ExpectedAnchor,
			}
			f.AddGap(newGap)
		}
	}

	return nil
}

// removeGap removes a gap from the pending list.
func (f *ChainGapFiller) removeGap(gap ChainGap) {
	f.gapsMu.Lock()
	defer f.gapsMu.Unlock()

	for i, g := range f.gaps {
		if g.ChainKey == gap.ChainKey && g.StartIndex == gap.StartIndex {
			// Remove by swapping with last element
			f.gaps[i] = f.gaps[len(f.gaps)-1]
			f.gaps = f.gaps[:len(f.gaps)-1]
			return
		}
	}
}

// Metrics returns filler metrics.
func (f *ChainGapFiller) Metrics() (gapsDetected, gapsFilled, entriesRecv uint64) {
	return f.gapsDetected.Load(), f.gapsFilled.Load(), f.entriesRecv.Load()
}

// TriggerCheck triggers an immediate gap check and fill cycle.
func (f *ChainGapFiller) TriggerCheck() {
	go f.checkAndFillGaps()
}
