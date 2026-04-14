// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const (
	// MinShardDepth is the minimum shard depth (2^2 = 4 shards).
	// Below this, parallelism gains are negligible.
	MinShardDepth = 2

	// MaxShardDepth is the maximum shard depth (2^8 = 256 shards).
	// Limited to 8 because routing uses only the first byte of the key hash.
	MaxShardDepth = 8

	// MinShardCount is the minimum number of shards (4).
	MinShardCount = 1 << MinShardDepth

	// MaxShardCount is the maximum number of shards (256).
	MaxShardCount = 1 << MaxShardDepth

	// DefaultShardCount is the recommended shard count for production (64).
	// With depth 6, this uses bits 7-2 of key[0] for routing.
	DefaultShardCount = 64
)

// ValidateShardCount checks that count is a valid shard count: positive,
// power of 2, and within [MinShardCount, MaxShardCount].
func ValidateShardCount(count int) error {
	if count <= 0 {
		return errors.BadRequest.WithFormat("shard count must be positive, got %d", count)
	}
	if count&(count-1) != 0 {
		return errors.BadRequest.WithFormat("shard count must be a power of 2, got %d", count)
	}
	if count < MinShardCount {
		return errors.BadRequest.WithFormat("shard count must be at least %d, got %d", MinShardCount, count)
	}
	if count > MaxShardCount {
		return errors.BadRequest.WithFormat("shard count must be at most %d, got %d", MaxShardCount, count)
	}
	return nil
}

// ShardDepthFromCount returns the bit depth for a given shard count.
// For example, 64 shards returns depth 6. The caller must validate count
// first with ValidateShardCount.
func ShardDepthFromCount(count int) int {
	return bits.TrailingZeros(uint(count))
}

// ShardCountFromDepth returns the shard count for a given depth.
// For example, depth 6 returns 64 shards.
func ShardCountFromDepth(depth int) int {
	return 1 << depth
}

// RouteToShard determines which shard a key belongs to based on the high-order
// bits of the first byte. This is the canonical routing function shared by both
// BPT sharding and transaction execution sharding.
//
// For depth d, it extracts the top d bits: key[0] >> (8 - d).
// This produces values in [0, 2^d).
func RouteToShard(keyHash [32]byte, shardDepth int) int {
	return int(keyHash[0] >> (8 - shardDepth))
}
