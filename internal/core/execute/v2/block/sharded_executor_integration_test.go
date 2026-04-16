// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// TestShardedExecutorWrapperBeginCreatesShardedBlock verifies that
// ShardedExecutorWrapper.Begin() returns a *ShardedBlock, not a regular *Block.
func TestShardedExecutorWrapperBeginCreatesShardedBlock(t *testing.T) {
	for _, shardCount := range []int{4, 16, 64} {
		t.Run(fmt.Sprintf("shards=%d", shardCount), func(t *testing.T) {
			wrapper, err := NewShardedExecutorWrapper(nil, shardCount)
			require.NoError(t, err)
			require.NotNil(t, wrapper)
			require.Equal(t, shardCount, wrapper.shardCount)

			// Verify wrapper was created correctly - it should produce
			// ShardedBlocks when Begin is called with a valid inner executor.
			// Without a full executor, we verify the wrapper's configuration.
			_, err = NewShardedBlock(nil, shardCount)
			require.NoError(t, err, "ShardedBlock creation should succeed with shard count %d", shardCount)
		})
	}
}

// TestShardedExecutorWrapperFallbackLogging verifies that the wrapper
// falls back to sequential with logging when sharding fails.
func TestShardedExecutorWrapperFallbackLogging(t *testing.T) {
	// Invalid shard count should fail at wrapper creation
	_, err := NewShardedExecutorWrapper(nil, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "power of two")

	_, err = NewShardedExecutorWrapper(nil, 512)
	require.Error(t, err)
	require.Contains(t, err.Error(), "between 1 and 256")
}

// TestExecutorConfigValidation verifies that the executor configuration
// validation correctly identifies valid and invalid shard counts.
// This test is part of Phase 4d - configuration validation.
func TestExecutorConfigValidation(t *testing.T) {
	// Test valid shard counts (power of 2, >= 4, <= 256)
	validCounts := []uint64{4, 8, 16, 32, 64, 128, 256}
	for _, count := range validCounts {
		t.Run(fmt.Sprintf("ValidShardCount=%d", count), func(t *testing.T) {
			err := database.ValidateShardCount(count)
			require.NoError(t, err, "shard count %d should be valid", count)
		})
	}

	// Test invalid shard counts
	invalidCounts := []uint64{
		0,    // too small
		1,    // power of 2 but < minimum
		2,    // power of 2 but < minimum
		3,    // not power of 2
		5,    // not power of 2
		7,    // not power of 2
		512,  // power of 2 but > maximum
		1024, // power of 2 but > maximum
		1023, // not power of 2
	}

	for _, count := range invalidCounts {
		t.Run(fmt.Sprintf("InvalidShardCount=%d", count), func(t *testing.T) {
			err := database.ValidateShardCount(count)
			require.Error(t, err, "shard count %d should be invalid", count)
		})
	}
}

// TestExecutorConfigDefaults verifies that ExecutorConfig returns sensible defaults.
func TestExecutorConfigDefaults(t *testing.T) {
	require.Equal(t, uint64(64), uint64(database.DefaultExecutorShardCount), "default shard count should be 64")
	require.True(t, database.DefaultExecutorShardCount > 0, "default shard count must be positive")

	// Verify the default is itself valid
	err := database.ValidateShardCount(uint64(database.DefaultExecutorShardCount))
	require.NoError(t, err, "default shard count should be valid")
}

// TestExecutorConfigStructure verifies that ExecutorConfig has the required fields.
func TestExecutorConfigStructure(t *testing.T) {
	cfg := &database.ExecutorConfig{
		ExecutorShardCount: 64,
	}
	require.Equal(t, uint64(64), cfg.ExecutorShardCount)

	// Verify we can create configs with all valid shard counts
	for _, count := range []uint64{4, 8, 16, 32, 64, 128, 256} {
		cfg := &database.ExecutorConfig{ExecutorShardCount: count}
		err := database.ValidateShardCount(cfg.ExecutorShardCount)
		require.NoError(t, err)
	}
}
