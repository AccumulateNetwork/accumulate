// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestHeightTracker_NewTracker(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)
	require.NotNil(t, tracker)
	assert.Equal(t, uint64(0), tracker.CurrentHeight())
	assert.Equal(t, uint64(1), tracker.NextHeight())
}

func TestHeightTracker_NewTrackerWithInitialHeight(t *testing.T) {
	tracker := adapter.NewHeightTracker(100)
	assert.Equal(t, uint64(100), tracker.CurrentHeight())
	assert.Equal(t, uint64(101), tracker.NextHeight())
}

func TestHeightTracker_RecordBlock(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	// Record first block
	height, err := tracker.RecordBlock(types.Round(2))
	require.NoError(t, err)
	assert.Equal(t, uint64(1), height)
	assert.Equal(t, uint64(1), tracker.CurrentHeight())

	// Record second block
	height, err = tracker.RecordBlock(types.Round(4))
	require.NoError(t, err)
	assert.Equal(t, uint64(2), height)
	assert.Equal(t, uint64(2), tracker.CurrentHeight())

	// Record third block (skipping round 6)
	height, err = tracker.RecordBlock(types.Round(8))
	require.NoError(t, err)
	assert.Equal(t, uint64(3), height)
}

func TestHeightTracker_RecordDuplicateRound(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, err := tracker.RecordBlock(types.Round(2))
	require.NoError(t, err)

	// Try to record same round again
	_, err = tracker.RecordBlock(types.Round(2))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already recorded")
}

func TestHeightTracker_HeightForRound(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))
	_, _ = tracker.RecordBlock(types.Round(8))

	// Check valid rounds
	height, ok := tracker.HeightForRound(types.Round(2))
	assert.True(t, ok)
	assert.Equal(t, uint64(1), height)

	height, ok = tracker.HeightForRound(types.Round(4))
	assert.True(t, ok)
	assert.Equal(t, uint64(2), height)

	height, ok = tracker.HeightForRound(types.Round(8))
	assert.True(t, ok)
	assert.Equal(t, uint64(3), height)

	// Check invalid round
	_, ok = tracker.HeightForRound(types.Round(6))
	assert.False(t, ok)
}

func TestHeightTracker_RoundForHeight(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))
	_, _ = tracker.RecordBlock(types.Round(8))

	// Check valid heights
	round, ok := tracker.RoundForHeight(1)
	assert.True(t, ok)
	assert.Equal(t, types.Round(2), round)

	round, ok = tracker.RoundForHeight(2)
	assert.True(t, ok)
	assert.Equal(t, types.Round(4), round)

	round, ok = tracker.RoundForHeight(3)
	assert.True(t, ok)
	assert.Equal(t, types.Round(8), round)

	// Check invalid height
	_, ok = tracker.RoundForHeight(4)
	assert.False(t, ok)
}

func TestHeightTracker_RoundRange(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))
	_, _ = tracker.RecordBlock(types.Round(6))
	_, _ = tracker.RecordBlock(types.Round(8))
	_, _ = tracker.RecordBlock(types.Round(10))

	// Get range [2, 4]
	rounds := tracker.RoundRange(2, 4)
	assert.Len(t, rounds, 3)
	assert.Equal(t, types.Round(4), rounds[0])
	assert.Equal(t, types.Round(6), rounds[1])
	assert.Equal(t, types.Round(8), rounds[2])

	// Get range [1, 5]
	rounds = tracker.RoundRange(1, 5)
	assert.Len(t, rounds, 5)

	// Get range beyond current height (should clamp)
	rounds = tracker.RoundRange(3, 100)
	assert.Len(t, rounds, 3) // heights 3, 4, 5

	// Invalid range
	rounds = tracker.RoundRange(10, 5)
	assert.Nil(t, rounds)

	// Empty range (beyond current)
	rounds = tracker.RoundRange(100, 200)
	assert.Nil(t, rounds)
}

func TestHeightTracker_HeightRange(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	// Empty tracker
	_, _, ok := tracker.HeightRange()
	assert.False(t, ok)

	// Add some blocks
	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))
	_, _ = tracker.RecordBlock(types.Round(6))

	minH, maxH, ok := tracker.HeightRange()
	assert.True(t, ok)
	assert.Equal(t, uint64(1), minH)
	assert.Equal(t, uint64(3), maxH)
}

func TestHeightTracker_HasRound(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))

	assert.True(t, tracker.HasRound(types.Round(2)))
	assert.True(t, tracker.HasRound(types.Round(4)))
	assert.False(t, tracker.HasRound(types.Round(3)))
	assert.False(t, tracker.HasRound(types.Round(6)))
}

func TestHeightTracker_HasHeight(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))

	assert.True(t, tracker.HasHeight(1))
	assert.True(t, tracker.HasHeight(2))
	assert.False(t, tracker.HasHeight(0))
	assert.False(t, tracker.HasHeight(3))
}

func TestHeightTracker_CommittedRounds(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(6))
	_, _ = tracker.RecordBlock(types.Round(4))

	rounds := tracker.CommittedRounds()
	assert.Len(t, rounds, 3)
	// Should be in order of commitment
	assert.Equal(t, types.Round(2), rounds[0])
	assert.Equal(t, types.Round(6), rounds[1])
	assert.Equal(t, types.Round(4), rounds[2])
}

func TestHeightTracker_BlockCount(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	assert.Equal(t, 0, tracker.BlockCount())

	_, _ = tracker.RecordBlock(types.Round(2))
	assert.Equal(t, 1, tracker.BlockCount())

	_, _ = tracker.RecordBlock(types.Round(4))
	assert.Equal(t, 2, tracker.BlockCount())
}

func TestHeightTracker_Reset(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))
	_, _ = tracker.RecordBlock(types.Round(6))

	assert.Equal(t, uint64(3), tracker.CurrentHeight())
	assert.Equal(t, 3, tracker.BlockCount())

	// Reset to new initial height
	tracker.Reset(100)

	assert.Equal(t, uint64(100), tracker.CurrentHeight())
	assert.Equal(t, 0, tracker.BlockCount())
	assert.False(t, tracker.HasRound(types.Round(2)))
	assert.False(t, tracker.HasHeight(1))
}

func TestHeightTracker_Restore(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	mappings := []adapter.RoundHeightMapping{
		{Round: types.Round(2), Height: 1},
		{Round: types.Round(4), Height: 2},
		{Round: types.Round(8), Height: 3},
	}

	err := tracker.Restore(mappings)
	require.NoError(t, err)

	assert.Equal(t, uint64(3), tracker.CurrentHeight())
	assert.Equal(t, 3, tracker.BlockCount())

	height, ok := tracker.HeightForRound(types.Round(4))
	assert.True(t, ok)
	assert.Equal(t, uint64(2), height)

	round, ok := tracker.RoundForHeight(3)
	assert.True(t, ok)
	assert.Equal(t, types.Round(8), round)
}

func TestHeightTracker_RestoreDuplicateRound(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	mappings := []adapter.RoundHeightMapping{
		{Round: types.Round(2), Height: 1},
		{Round: types.Round(2), Height: 2}, // Duplicate round
	}

	err := tracker.Restore(mappings)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate round")
}

func TestHeightTracker_RestoreDuplicateHeight(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	mappings := []adapter.RoundHeightMapping{
		{Round: types.Round(2), Height: 1},
		{Round: types.Round(4), Height: 1}, // Duplicate height
	}

	err := tracker.Restore(mappings)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate height")
}

func TestHeightTracker_Snapshot(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	_, _ = tracker.RecordBlock(types.Round(2))
	_, _ = tracker.RecordBlock(types.Round(4))
	_, _ = tracker.RecordBlock(types.Round(8))

	snapshot := tracker.Snapshot()
	assert.Len(t, snapshot, 3)

	// Verify snapshot contents
	assert.Equal(t, types.Round(2), snapshot[0].Round)
	assert.Equal(t, uint64(1), snapshot[0].Height)
	assert.Equal(t, types.Round(4), snapshot[1].Round)
	assert.Equal(t, uint64(2), snapshot[1].Height)
	assert.Equal(t, types.Round(8), snapshot[2].Round)
	assert.Equal(t, uint64(3), snapshot[2].Height)
}

func TestHeightTracker_SnapshotAndRestore(t *testing.T) {
	tracker1 := adapter.NewHeightTracker(0)

	_, _ = tracker1.RecordBlock(types.Round(2))
	_, _ = tracker1.RecordBlock(types.Round(4))
	_, _ = tracker1.RecordBlock(types.Round(8))

	snapshot := tracker1.Snapshot()

	// Create new tracker and restore
	tracker2 := adapter.NewHeightTracker(0)
	err := tracker2.Restore(snapshot)
	require.NoError(t, err)

	// Verify both trackers have same state
	assert.Equal(t, tracker1.CurrentHeight(), tracker2.CurrentHeight())
	assert.Equal(t, tracker1.BlockCount(), tracker2.BlockCount())

	for _, round := range []types.Round{2, 4, 8} {
		h1, ok1 := tracker1.HeightForRound(round)
		h2, ok2 := tracker2.HeightForRound(round)
		assert.Equal(t, ok1, ok2)
		assert.Equal(t, h1, h2)
	}
}

func TestHeightTracker_ConcurrentAccess(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	var wg sync.WaitGroup
	rounds := make(chan types.Round, 100)

	// Generate rounds to record
	for i := types.Round(2); i <= 200; i += 2 {
		rounds <- i
	}
	close(rounds)

	// Record blocks concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := range rounds {
				_, _ = tracker.RecordBlock(round)
			}
		}()
	}

	// Read concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tracker.CurrentHeight()
				tracker.NextHeight()
				tracker.HeightForRound(types.Round(4))
				tracker.RoundForHeight(1)
				tracker.HasRound(types.Round(2))
				tracker.HasHeight(1)
			}
		}()
	}

	wg.Wait()

	// Verify no blocks were lost
	assert.Equal(t, 100, tracker.BlockCount())
}

func TestHeightTracker_SequentialHeightsNoGaps(t *testing.T) {
	tracker := adapter.NewHeightTracker(0)

	// Record blocks with non-sequential rounds
	rounds := []types.Round{2, 6, 8, 14, 16, 22, 24, 30}

	for _, r := range rounds {
		_, err := tracker.RecordBlock(r)
		require.NoError(t, err)
	}

	// Verify heights are sequential with no gaps
	for i := uint64(1); i <= uint64(len(rounds)); i++ {
		assert.True(t, tracker.HasHeight(i), "missing height %d", i)
	}

	// Verify no extra heights
	assert.False(t, tracker.HasHeight(uint64(len(rounds)+1)))
	assert.False(t, tracker.HasHeight(0))
}

func TestBlockMetadata(t *testing.T) {
	certDigest := types.CertificateDigest{1, 2, 3, 4}
	meta := adapter.NewBlockMetadata(10, types.Round(20), certDigest)

	assert.Equal(t, uint64(10), meta.Height)
	assert.Equal(t, types.Round(20), meta.Round)
	assert.Equal(t, certDigest, meta.CertificateDigest)
	assert.Equal(t, types.Round(0), meta.PreviousRound)

	// Add previous round
	meta.WithPreviousRound(types.Round(18))
	assert.Equal(t, types.Round(18), meta.PreviousRound)
}

func TestBlockMetadata_ChainedBlocks(t *testing.T) {
	// Simulate a chain of blocks
	blocks := make([]*adapter.BlockMetadata, 5)

	for i := 0; i < 5; i++ {
		round := types.Round((i + 1) * 2)
		certDigest := types.CertificateDigest{byte(i)}

		blocks[i] = adapter.NewBlockMetadata(uint64(i+1), round, certDigest)
		if i > 0 {
			blocks[i].WithPreviousRound(blocks[i-1].Round)
		}
	}

	// Verify chain
	for i := 1; i < 5; i++ {
		assert.Equal(t, blocks[i-1].Round, blocks[i].PreviousRound)
	}
}
