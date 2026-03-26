// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TestShardRouting verifies that keys are routed to the correct shards based
// on their high-order bits.
func TestShardRouting(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	// Test with 4-bit depth (16 shards)
	s, err := NewShardedBPT(store, key, 4)
	require.NoError(t, err)
	require.Equal(t, 16, s.numShards)

	// Test routing for each shard
	for i := 0; i < 16; i++ {
		// Create a key that should route to shard i
		var keyHash [32]byte
		keyHash[0] = byte(i << 4) // Put shard ID in high 4 bits

		shardID, shard := s.routeToShard(keyHash)
		require.Equal(t, i, shardID, "key with prefix %x should route to shard %d", keyHash[0], i)
		require.Equal(t, s.shards[i], shard, "key with prefix %x should route to shard %d", keyHash[0], i)
	}

	// Test with 5-bit depth (32 shards)
	s5, err := NewShardedBPT(store, key, 5)
	require.NoError(t, err)
	require.Equal(t, 32, s5.numShards)

	// Verify routing for 32 shards
	for i := 0; i < 32; i++ {
		var keyHash [32]byte
		keyHash[0] = byte(i << 3) // Put shard ID in high 5 bits

		shardID, shard := s5.routeToShard(keyHash)
		require.Equal(t, i, shardID, "key with prefix %x should route to shard %d", keyHash[0], i)
		require.Equal(t, s5.shards[i], shard, "key with prefix %x should route to shard %d", keyHash[0], i)
	}
}

// TestRootHashEquivalence is the CRITICAL test that proves correctness.
// It verifies that a ShardedBPT produces the exact same root hash as a
// non-sharded BPT when given identical data.
func TestRootHashEquivalence(t *testing.T) {
	tests := []struct {
		name       string
		shardDepth int
		numEntries int
	}{
		{"16 shards, 100 entries", 4, 100},
		{"32 shards, 100 entries", 5, 100},
		{"16 shards, 1000 entries", 4, 1000},
		{"4 shards, 256 entries", 2, 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create two separate stores for independent BPTs
			kvs1 := memory.New(nil).Begin(nil, true)
			defer kvs1.Discard()
			store1 := keyvalue.RecordStore{Store: kvs1}

			kvs2 := memory.New(nil).Begin(nil, true)
			defer kvs2.Discard()
			store2 := keyvalue.RecordStore{Store: kvs2}

			key := record.NewKey("test")

			// Create sharded and non-sharded BPTs
			sharded, err := NewShardedBPT(store1, key, tt.shardDepth)
			require.NoError(t, err)

			nonSharded := New(nil, nil, store2, key)

			// Insert the same data into both
			entries := make([]*record.Key, tt.numEntries)
			for i := 0; i < tt.numEntries; i++ {
				k := record.NewKey(fmt.Sprintf("key-%d", i))
				v := sha256.Sum256([]byte(fmt.Sprintf("value-%d", i)))

				entries[i] = k
				err = sharded.Insert(k, v[:])
				require.NoError(t, err)
				err = nonSharded.Insert(k, v[:])
				require.NoError(t, err)
			}

			// Get root hashes from both
			shardedRoot, err := sharded.GetRootHash()
			require.NoError(t, err)
			nonShardedRoot, err := nonSharded.GetRootHash()
			require.NoError(t, err)

			// CRITICAL: Root hashes must be identical
			require.Equal(t, nonShardedRoot, shardedRoot,
				"sharded and non-sharded BPTs must produce identical root hashes")

			// Verify we can retrieve all entries from both
			for _, k := range entries {
				shardedVal, err := sharded.Get(k)
				require.NoError(t, err)
				nonShardedVal, err := nonSharded.Get(k)
				require.NoError(t, err)
				require.Equal(t, nonShardedVal, shardedVal,
					"sharded and non-sharded BPTs must return identical values")
			}
		})
	}
}

// TestConcurrentInserts verifies that concurrent inserts are safe with the
// race detector enabled. This is the stress test for thread safety.
func TestConcurrentInserts(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	// Create sharded BPT with 16 shards
	s, err := NewShardedBPT(store, key, 4)
	require.NoError(t, err)

	// Number of goroutines and entries per goroutine
	numGoroutines := 64
	entriesPerGoroutine := 100

	// Track all keys for verification
	allKeys := make([][]*record.Key, numGoroutines)
	for i := range allKeys {
		allKeys[i] = make([]*record.Key, entriesPerGoroutine)
	}

	// Insert concurrently from multiple goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		g := g
		go func() {
			defer wg.Done()

			for i := 0; i < entriesPerGoroutine; i++ {
				k := record.NewKey(fmt.Sprintf("goroutine-%d-key-%d", g, i))
				v := sha256.Sum256([]byte(fmt.Sprintf("value-%d-%d", g, i)))

				allKeys[g][i] = k
				err := s.Insert(k, v[:])
				require.NoError(t, err)
			}
		}()
	}

	wg.Wait()

	// Verify all entries are present
	for g := 0; g < numGoroutines; g++ {
		for i := 0; i < entriesPerGoroutine; i++ {
			k := allKeys[g][i]
			val, err := s.Get(k)
			require.NoError(t, err)

			expectedVal := sha256.Sum256([]byte(fmt.Sprintf("value-%d-%d", g, i)))
			require.Equal(t, expectedVal[:], val,
				"value mismatch for key from goroutine %d, entry %d", g, i)
		}
	}

	// Verify we can get a root hash without errors
	_, err = s.GetRootHash()
	require.NoError(t, err)
}

// TestEmptyShards verifies that the root hash computation correctly handles
// empty shards (shards with no data).
func TestEmptyShards(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	// Create sharded BPT with 16 shards
	s, err := NewShardedBPT(store, key, 4)
	require.NoError(t, err)

	// Insert data into only a few shards (not all)
	// Keys starting with 0x0X will go to shard 0
	// Keys starting with 0x1X will go to shard 1
	testKeys := []string{
		"key-shard-0-a",
		"key-shard-0-b",
		"key-shard-1-a",
	}

	for _, k := range testKeys {
		key := record.NewKey(k)
		v := sha256.Sum256([]byte(k))
		err = s.Insert(key, v[:])
		require.NoError(t, err)
	}

	// Get root hash - should handle empty shards correctly
	rootHash, err := s.GetRootHash()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, rootHash, "root hash should not be empty")

	// Verify retrieval works
	for _, k := range testKeys {
		key := record.NewKey(k)
		val, err := s.Get(key)
		require.NoError(t, err)

		expectedVal := sha256.Sum256([]byte(k))
		require.Equal(t, expectedVal[:], val)
	}
}

// TestHashBranch verifies that hashBranch follows the same semantics as
// BPT's branch.getHash() method.
func TestHashBranch(t *testing.T) {
	left := sha256.Sum256([]byte("left"))
	right := sha256.Sum256([]byte("right"))
	empty := [32]byte{}

	// Test both non-empty
	combined := hashBranch(left, right)
	require.NotEqual(t, empty, combined)
	require.NotEqual(t, left, combined)
	require.NotEqual(t, right, combined)

	// Verify it matches manual concatenation and hash
	var b [64]byte
	copy(b[:32], left[:])
	copy(b[32:], right[:])
	expected := sha256.Sum256(b[:])
	require.Equal(t, expected, combined)

	// Test only left
	leftOnly := hashBranch(left, empty)
	require.Equal(t, left, leftOnly)

	// Test only right
	rightOnly := hashBranch(empty, right)
	require.Equal(t, right, rightOnly)

	// Test both empty
	bothEmpty := hashBranch(empty, empty)
	require.Equal(t, empty, bothEmpty)
}

// TestCombineShardRoots verifies the hierarchical combining algorithm.
func TestCombineShardRoots(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	s, err := NewShardedBPT(store, key, 4)
	require.NoError(t, err)

	// Test with various numbers of roots
	tests := []struct {
		name     string
		numRoots int
	}{
		{"1 root", 1},
		{"2 roots", 2},
		{"4 roots", 4},
		{"8 roots", 8},
		{"16 roots", 16},
		{"3 roots (odd)", 3},
		{"7 roots (odd)", 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := make([][32]byte, tt.numRoots)
			for i := 0; i < tt.numRoots; i++ {
				roots[i] = sha256.Sum256([]byte(fmt.Sprintf("root-%d", i)))
			}

			result := s.combineShardRoots(roots)
			require.NotEqual(t, [32]byte{}, result, "combined root should not be empty")

			// For single root, result should be the root itself
			if tt.numRoots == 1 {
				require.Equal(t, roots[0], result)
			}
		})
	}
}

// TestShardedBPTDelete verifies that delete operations work correctly.
func TestShardedBPTDelete(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	s, err := NewShardedBPT(store, key, 4)
	require.NoError(t, err)

	// Insert some entries
	keys := []string{"key1", "key2", "key3"}
	for _, k := range keys {
		key := record.NewKey(k)
		v := sha256.Sum256([]byte(k))
		err = s.Insert(key, v[:])
		require.NoError(t, err)
	}

	// Verify entries exist
	for _, k := range keys {
		key := record.NewKey(k)
		_, err := s.Get(key)
		require.NoError(t, err)
	}

	// Delete one entry
	key1 := record.NewKey("key1")
	err = s.Delete(key1)
	require.NoError(t, err)

	// Verify it's gone
	_, err = s.Get(key1)
	require.Error(t, err)

	// Verify others still exist
	for _, k := range keys[1:] {
		key := record.NewKey(k)
		val, err := s.Get(key)
		require.NoError(t, err)

		expectedVal := sha256.Sum256([]byte(k))
		require.Equal(t, expectedVal[:], val)
	}
}

// TestInvalidShardDepth verifies that invalid shard depths are rejected.
func TestInvalidShardDepth(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	// Test depths that are too small or too large
	invalidDepths := []int{0, -1, 9, 10}
	for _, depth := range invalidDepths {
		_, err := NewShardedBPT(store, key, depth)
		require.Error(t, err, "depth %d should be rejected", depth)
	}

	// Test valid depths
	validDepths := []int{1, 2, 3, 4, 5, 6, 7, 8}
	for _, depth := range validDepths {
		_, err := NewShardedBPT(store, key, depth)
		require.NoError(t, err, "depth %d should be accepted", depth)
	}
}
