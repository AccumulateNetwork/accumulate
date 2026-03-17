// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// HeightTracker maps DAG-BFT consensus rounds to sequential block heights.
// It maintains a bidirectional mapping between rounds and heights, ensuring
// that block heights are always sequential with no gaps, regardless of
// skipped leader rounds in the DAG.
type HeightTracker struct {
	mu sync.RWMutex

	// currentHeight is the latest committed block height.
	currentHeight uint64

	// roundToHeight maps leader rounds to their corresponding block heights.
	// Only rounds that produced committed blocks are tracked.
	roundToHeight map[types.Round]uint64

	// heightToRound maps block heights to the leader rounds that produced them.
	heightToRound map[uint64]types.Round

	// roundHistory stores the ordered list of committed rounds for range queries.
	roundHistory []types.Round
}

// NewHeightTracker creates a new HeightTracker starting at the given height.
// The initialHeight is typically the last committed block height from state recovery.
func NewHeightTracker(initialHeight uint64) *HeightTracker {
	return &HeightTracker{
		currentHeight: initialHeight,
		roundToHeight: make(map[types.Round]uint64),
		heightToRound: make(map[uint64]types.Round),
		roundHistory:  make([]types.Round, 0),
	}
}

// CurrentHeight returns the current (latest) block height.
func (h *HeightTracker) CurrentHeight() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.currentHeight
}

// NextHeight returns the height that will be assigned to the next block.
func (h *HeightTracker) NextHeight() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.currentHeight + 1
}

// RecordBlock records a new block from a leader round and returns the assigned height.
// This increments the current height and creates the bidirectional mapping.
// Returns an error if the round has already been recorded.
func (h *HeightTracker) RecordBlock(round types.Round) (uint64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if round was already recorded
	if _, exists := h.roundToHeight[round]; exists {
		return 0, fmt.Errorf("round %d already recorded", round)
	}

	// Increment height
	h.currentHeight++
	height := h.currentHeight

	// Create bidirectional mappings
	h.roundToHeight[round] = height
	h.heightToRound[height] = round
	h.roundHistory = append(h.roundHistory, round)

	return height, nil
}

// HeightForRound returns the block height for a given leader round.
// Returns 0 and false if the round has not been recorded.
func (h *HeightTracker) HeightForRound(round types.Round) (uint64, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	height, exists := h.roundToHeight[round]
	return height, exists
}

// RoundForHeight returns the leader round that produced the block at the given height.
// Returns 0 and false if no block exists at that height.
func (h *HeightTracker) RoundForHeight(height uint64) (types.Round, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	round, exists := h.heightToRound[height]
	return round, exists
}

// RoundRange returns all rounds that were committed within a height range [startHeight, endHeight].
// The rounds are returned in order of their corresponding block heights.
func (h *HeightTracker) RoundRange(startHeight, endHeight uint64) []types.Round {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if startHeight > endHeight || startHeight > h.currentHeight {
		return nil
	}

	// Clamp endHeight to currentHeight
	if endHeight > h.currentHeight {
		endHeight = h.currentHeight
	}

	rounds := make([]types.Round, 0, endHeight-startHeight+1)
	for height := startHeight; height <= endHeight; height++ {
		if round, exists := h.heightToRound[height]; exists {
			rounds = append(rounds, round)
		}
	}

	return rounds
}

// HeightRange returns the range of heights [minHeight, maxHeight] that are tracked.
// Returns (0, 0, false) if no blocks have been recorded.
func (h *HeightTracker) HeightRange() (minHeight, maxHeight uint64, ok bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.roundHistory) == 0 {
		return 0, 0, false
	}

	// Find minimum height in the map
	minHeight = h.currentHeight
	for height := range h.heightToRound {
		if height < minHeight {
			minHeight = height
		}
	}

	return minHeight, h.currentHeight, true
}

// HasRound returns true if the given round has been recorded.
func (h *HeightTracker) HasRound(round types.Round) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	_, exists := h.roundToHeight[round]
	return exists
}

// HasHeight returns true if a block has been recorded at the given height.
func (h *HeightTracker) HasHeight(height uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	_, exists := h.heightToRound[height]
	return exists
}

// CommittedRounds returns all rounds that have been committed, in order.
func (h *HeightTracker) CommittedRounds() []types.Round {
	h.mu.RLock()
	defer h.mu.RUnlock()

	rounds := make([]types.Round, len(h.roundHistory))
	copy(rounds, h.roundHistory)
	return rounds
}

// BlockCount returns the number of blocks that have been recorded.
func (h *HeightTracker) BlockCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.roundHistory)
}

// Reset clears all tracking data and resets to the given initial height.
// This should only be used during state recovery or testing.
func (h *HeightTracker) Reset(initialHeight uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.currentHeight = initialHeight
	h.roundToHeight = make(map[types.Round]uint64)
	h.heightToRound = make(map[uint64]types.Round)
	h.roundHistory = make([]types.Round, 0)
}

// Restore restores the height tracker state from persisted mappings.
// This is used during node restart to recover the round-height mappings.
func (h *HeightTracker) Restore(mappings []RoundHeightMapping) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Clear existing state
	h.roundToHeight = make(map[types.Round]uint64)
	h.heightToRound = make(map[uint64]types.Round)
	h.roundHistory = make([]types.Round, 0, len(mappings))

	var maxHeight uint64
	for _, m := range mappings {
		// Check for duplicates
		if _, exists := h.roundToHeight[m.Round]; exists {
			return fmt.Errorf("duplicate round %d in mappings", m.Round)
		}
		if _, exists := h.heightToRound[m.Height]; exists {
			return fmt.Errorf("duplicate height %d in mappings", m.Height)
		}

		h.roundToHeight[m.Round] = m.Height
		h.heightToRound[m.Height] = m.Round
		h.roundHistory = append(h.roundHistory, m.Round)

		if m.Height > maxHeight {
			maxHeight = m.Height
		}
	}

	h.currentHeight = maxHeight
	return nil
}

// Snapshot returns the current state as a slice of mappings for persistence.
func (h *HeightTracker) Snapshot() []RoundHeightMapping {
	h.mu.RLock()
	defer h.mu.RUnlock()

	mappings := make([]RoundHeightMapping, 0, len(h.roundHistory))
	for _, round := range h.roundHistory {
		if height, exists := h.roundToHeight[round]; exists {
			mappings = append(mappings, RoundHeightMapping{
				Round:  round,
				Height: height,
			})
		}
	}

	return mappings
}

// RoundHeightMapping represents a mapping between a round and a height.
// This is used for persistence and restoration of the tracker state.
type RoundHeightMapping struct {
	Round  types.Round
	Height uint64
}

// BlockMetadata contains block information that combines DAG-BFT round
// information with traditional block height for compatibility.
type BlockMetadata struct {
	// Height is the sequential block height (1, 2, 3, ...).
	Height uint64

	// Round is the DAG-BFT leader round that produced this block.
	Round types.Round

	// PreviousRound is the round of the previous block (for chain traversal).
	PreviousRound types.Round

	// CertificateDigest is the digest of the certificate that committed this block.
	CertificateDigest types.CertificateDigest
}

// NewBlockMetadata creates metadata for a new block.
func NewBlockMetadata(height uint64, round types.Round, certDigest types.CertificateDigest) *BlockMetadata {
	return &BlockMetadata{
		Height:            height,
		Round:             round,
		CertificateDigest: certDigest,
	}
}

// WithPreviousRound sets the previous round reference.
func (m *BlockMetadata) WithPreviousRound(prevRound types.Round) *BlockMetadata {
	m.PreviousRound = prevRound
	return m
}
