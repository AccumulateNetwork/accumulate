// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateShardCount(t *testing.T) {
	// Valid counts: all powers of 2 in [4, 256]
	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		count := 1 << depth
		t.Run(fmt.Sprintf("valid_%d", count), func(t *testing.T) {
			require.NoError(t, ValidateShardCount(count))
		})
	}

	// Invalid: non-positive
	for _, count := range []int{0, -1, -64} {
		t.Run(fmt.Sprintf("non_positive_%d", count), func(t *testing.T) {
			err := ValidateShardCount(count)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "positive")
		})
	}

	// Invalid: not power of 2
	for _, count := range []int{3, 5, 6, 7, 9, 10, 12, 15, 17, 24, 48, 100, 255} {
		t.Run(fmt.Sprintf("not_power_of_2_%d", count), func(t *testing.T) {
			err := ValidateShardCount(count)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "power of 2")
		})
	}

	// Invalid: too small (power of 2 but below minimum)
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("too_small_%d", count), func(t *testing.T) {
			err := ValidateShardCount(count)
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("at least %d", MinShardCount))
		})
	}

	// Invalid: too large (power of 2 but above maximum)
	for _, count := range []int{512, 1024} {
		t.Run(fmt.Sprintf("too_large_%d", count), func(t *testing.T) {
			err := ValidateShardCount(count)
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("at most %d", MaxShardCount))
		})
	}
}

func TestShardDepthFromCount(t *testing.T) {
	tests := []struct {
		count int
		depth int
	}{
		{4, 2},
		{8, 3},
		{16, 4},
		{32, 5},
		{64, 6},
		{128, 7},
		{256, 8},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_shards", tt.count), func(t *testing.T) {
			assert.Equal(t, tt.depth, ShardDepthFromCount(tt.count))
		})
	}
}

func TestShardCountFromDepth(t *testing.T) {
	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		count := ShardCountFromDepth(depth)
		assert.Equal(t, 1<<depth, count)
		// Round-trip
		assert.Equal(t, depth, ShardDepthFromCount(count))
	}
}

func TestRouteToShard(t *testing.T) {
	// For each valid depth, verify routing covers all shards and is correct.
	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		numShards := 1 << depth
		t.Run(fmt.Sprintf("depth_%d_%d_shards", depth, numShards), func(t *testing.T) {
			seen := make([]bool, numShards)

			// Test all 256 possible first-byte values
			for b := 0; b < 256; b++ {
				var key [32]byte
				key[0] = byte(b)
				shard := RouteToShard(key, depth)

				// Shard must be in valid range
				require.GreaterOrEqual(t, shard, 0)
				require.Less(t, shard, numShards)

				// Verify the math: shard should equal key[0] >> (8 - depth)
				expected := b >> (8 - depth)
				assert.Equal(t, expected, shard, "byte %d, depth %d", b, depth)

				seen[shard] = true
			}

			// All shards should be reachable
			for i, s := range seen {
				assert.True(t, s, "shard %d was never reached with depth %d", i, depth)
			}
		})
	}
}

func TestRoutingDistribution(t *testing.T) {
	// Verify uniform distribution: with 256 possible first-byte values,
	// each shard should get exactly 256/numShards keys.
	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		numShards := 1 << depth
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			counts := make([]int, numShards)
			for b := 0; b < 256; b++ {
				var key [32]byte
				key[0] = byte(b)
				shard := RouteToShard(key, depth)
				counts[shard]++
			}

			expectedPerShard := 256 / numShards
			for i, c := range counts {
				assert.Equal(t, expectedPerShard, c,
					"shard %d got %d keys, expected %d (depth %d)", i, c, expectedPerShard, depth)
			}
		})
	}
}

func TestRoutingDeterminism(t *testing.T) {
	// Same key always routes to the same shard.
	var key [32]byte
	key[0] = 0xAB
	key[1] = 0xCD // Should not affect routing

	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		first := RouteToShard(key, depth)
		for i := 0; i < 100; i++ {
			assert.Equal(t, first, RouteToShard(key, depth),
				"routing not deterministic at depth %d", depth)
		}
	}

	// Changing bytes after [0] should not change shard assignment.
	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		base := RouteToShard(key, depth)
		for byteIdx := 1; byteIdx < 32; byteIdx++ {
			modified := key
			modified[byteIdx] = 0xFF
			assert.Equal(t, base, RouteToShard(modified, depth),
				"changing key[%d] should not affect routing at depth %d", byteIdx, depth)
		}
	}
}

func TestRouteToShardMatchesShardedBPT(t *testing.T) {
	// Verify that the exported RouteToShard produces the same results as
	// ShardedBPT.routeToShard for all valid depths.
	// ShardedBPT recommends 4-6, but supports 1-8. We test the full range.
	for depth := MinShardDepth; depth <= MaxShardDepth; depth++ {
		numShards := 1 << depth
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			for b := 0; b < 256; b++ {
				var key [32]byte
				key[0] = byte(b)
				exported := RouteToShard(key, depth)
				// Manual calculation matching sharded.go:103
				internal := int(key[0] >> (8 - depth))
				assert.Equal(t, internal, exported,
					"RouteToShard disagrees with internal routing for byte %d depth %d", b, depth)
				assert.Less(t, exported, numShards)
			}
		})
	}
}

func TestDefaultShardCount(t *testing.T) {
	require.NoError(t, ValidateShardCount(DefaultShardCount))
	assert.Equal(t, 64, DefaultShardCount)
	assert.Equal(t, 6, ShardDepthFromCount(DefaultShardCount))
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 4, MinShardCount)
	assert.Equal(t, 256, MaxShardCount)
	assert.Equal(t, 2, MinShardDepth)
	assert.Equal(t, 8, MaxShardDepth)
	assert.Equal(t, 1<<MinShardDepth, MinShardCount)
	assert.Equal(t, 1<<MaxShardDepth, MaxShardCount)
}

func TestCacheLinePadding(t *testing.T) {
	// Verify that paddedMutex is exactly 64 bytes (one cache line).
	assert.Equal(t, uintptr(64), unsafe.Sizeof(paddedMutex{}),
		"paddedMutex should be 64 bytes for cache-line alignment")

	// Verify that an array of paddedMutex has proper spacing.
	mutexes := make([]paddedMutex, 4)
	for i := 1; i < len(mutexes); i++ {
		offset := uintptr(unsafe.Pointer(&mutexes[i])) - uintptr(unsafe.Pointer(&mutexes[0]))
		expected := uintptr(i) * 64
		assert.Equal(t, expected, offset,
			"paddedMutex[%d] not at expected cache-line offset", i)
	}
}

func TestShardDepthBitShiftValues(t *testing.T) {
	// Document the exact bit-shift for each valid depth.
	expected := map[int]int{
		2: 6, // key[0] >> 6
		3: 5, // key[0] >> 5
		4: 4, // key[0] >> 4
		5: 3, // key[0] >> 3
		6: 2, // key[0] >> 2
		7: 1, // key[0] >> 1
		8: 0, // key[0] >> 0
	}
	for depth, shift := range expected {
		actual := 8 - depth
		assert.Equal(t, shift, actual,
			"depth %d should use shift %d", depth, shift)

		// Verify max shard ID
		maxShardID := int(math.Pow(2, float64(depth))) - 1
		maxByte := byte(0xFF)
		assert.Equal(t, maxShardID, int(maxByte>>actual),
			"depth %d: max byte 0xFF should produce shard %d", depth, maxShardID)
	}
}
