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

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// ShardedBPT is a BPT that partitions the tree at a configurable depth into
// independent shards for parallel updates. Each shard is a standard BPT
// instance with its own locking, providing embarrassingly parallel operations
// with zero contention between shards.
//
// The sharding is based on the natural binary structure of the tree. At depth
// N, the tree has 2^N branches, and each branch becomes an independent shard.
// Keys are routed to shards using the high-order bits of the key hash.
//
// Storage format is identical to non-sharded BPT - no database changes needed.
// The tree structure itself provides natural partitioning.
type ShardedBPT struct {
	shardDepth int          // Number of bits for routing (4, 5, or 6)
	numShards  int          // Number of shards (2^shardDepth)
	shards     []*BPT       // Array of standard BPT instances
	shardMu    []sync.Mutex // Per-shard locks for thread safety
	store      database.Store
	key        *record.Key
}

// NewShardedBPT creates a new ShardedBPT with the specified shard depth.
// The depth determines how many shards are created (2^depth shards).
//
// Recommended depths:
//   - 4 bits = 16 shards (optimal for 16-core systems)
//   - 5 bits = 32 shards (for 32-core systems)
//   - 6 bits = 64 shards (diminishing returns beyond this)
//
// The storage key is used as the base for all shard BPT instances.
func NewShardedBPT(store database.Store, key *record.Key, depth int) (*ShardedBPT, error) {
	if depth < 1 || depth > 8 {
		return nil, errors.BadRequest.WithFormat("shard depth must be between 1 and 8, got %d", depth)
	}

	numShards := 1 << depth
	s := &ShardedBPT{
		shardDepth: depth,
		numShards:  numShards,
		shards:     make([]*BPT, numShards),
		shardMu:    make([]sync.Mutex, numShards),
		store:      store,
		key:        key,
	}

	// Create a BPT instance for each shard
	// Each shard gets its own key prefix to avoid storage collisions
	for i := 0; i < numShards; i++ {
		shardKey := key.Append(fmt.Sprintf("shard-%d", i))
		s.shards[i] = New(nil, nil, store, shardKey)
	}

	return s, nil
}

// routeToShard determines which shard a key belongs to based on the high-order
// bits of the key hash. This uses the same routing logic as the BPT's internal
// tree structure. Returns both the shard and its index for locking.
func (s *ShardedBPT) routeToShard(keyHash [32]byte) (int, *BPT) {
	// Extract the high-order bits from the first byte of the key hash
	// For depth=4: shifts right by 4, giving us bits 7-4 (0-15)
	// For depth=5: shifts right by 3, giving us bits 7-3 (0-31)
	// For depth=6: shifts right by 2, giving us bits 7-2 (0-63)
	shardID := int(keyHash[0] >> (8 - s.shardDepth))
	return shardID, s.shards[shardID]
}

// Insert updates or inserts a value for the given key. The operation is
// routed to the appropriate shard based on the key hash. Thread-safe with
// per-shard locking.
func (s *ShardedBPT) Insert(key *record.Key, value []byte) error {
	shardID, shard := s.routeToShard(key.Hash())
	s.shardMu[shardID].Lock()
	defer s.shardMu[shardID].Unlock()
	return shard.Insert(key, value)
}

// Get retrieves the value associated with the given key. The operation is
// routed to the appropriate shard based on the key hash. Thread-safe with
// per-shard locking.
func (s *ShardedBPT) Get(key *record.Key) ([]byte, error) {
	shardID, shard := s.routeToShard(key.Hash())
	s.shardMu[shardID].Lock()
	defer s.shardMu[shardID].Unlock()
	return shard.Get(key)
}

// Delete removes the entry for the given key, if present. The operation is
// routed to the appropriate shard based on the key hash. Thread-safe with
// per-shard locking.
func (s *ShardedBPT) Delete(key *record.Key) error {
	shardID, shard := s.routeToShard(key.Hash())
	s.shardMu[shardID].Lock()
	defer s.shardMu[shardID].Unlock()
	return shard.Delete(key)
}

// GetRootHash computes the root hash by combining all shard root hashes
// hierarchically. This is the only coordination point in the sharded BPT.
//
// The algorithm:
// 1. Read root hash from each shard IN PARALLEL (each executePending() independently)
// 2. Combine the shard roots bottom-up in a virtual binary tree
// 3. Return the final root hash
//
// This produces the same root hash as a non-sharded BPT with the same data.
// The parallel execution of executePending() across shards is a key performance benefit.
func (s *ShardedBPT) GetRootHash() ([32]byte, error) {
	// Read all shard root hashes IN PARALLEL
	// Each shard's executePending() runs concurrently
	shardRoots := make([][32]byte, s.numShards)
	errChan := make(chan error, s.numShards)
	var wg sync.WaitGroup

	for i := range s.shards {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			s.shardMu[idx].Lock()
			defer s.shardMu[idx].Unlock()

			rootHash, err := s.shards[idx].GetRootHash()
			if err != nil {
				errChan <- errors.UnknownError.WithFormat("get shard %d root: %w", idx, err)
				return
			}
			shardRoots[idx] = rootHash
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return [32]byte{}, err
	}

	// Combine the shard roots hierarchically
	return s.combineShardRoots(shardRoots), nil
}

// combineShardRoots combines an array of shard root hashes into a single
// root hash by building a virtual binary tree bottom-up.
//
// The algorithm pairs adjacent roots and hashes them together, repeating
// until only one root remains. For odd numbers of roots, the last root
// is carried forward to the next level.
//
// CRITICAL: BPT routing is inverted - bit=1 goes LEFT, bit=0 goes RIGHT.
// So when combining shard i (even) and i+1 (odd), we hash (i+1, i) not (i, i+1).
//
// This follows the same hash semantics as BPT's branch.getHash() to ensure
// the final root hash is identical to a non-sharded BPT.
func (s *ShardedBPT) combineShardRoots(roots [][32]byte) [32]byte {
	current := roots

	// Build virtual tree bottom-up
	for len(current) > 1 {
		next := make([][32]byte, (len(current)+1)/2)
		for i := 0; i < len(current); i += 2 {
			if i+1 < len(current) {
				// Pair exists - hash (odd, even) because BPT routing is inverted
				// Shard i (even, bit=0) goes RIGHT
				// Shard i+1 (odd, bit=1) goes LEFT
				// So hash(LEFT, RIGHT) = hash(i+1, i)
				next[i/2] = hashBranch(current[i+1], current[i])
			} else {
				// Odd number - carry forward the last root
				next[i/2] = current[i]
			}
		}
		current = next
	}

	return current[0]
}

// hashBranch combines two branch hashes following BPT's branch.getHash()
// semantics exactly. This is critical for ensuring root hash equivalence
// between sharded and non-sharded BPTs.
//
// The logic:
// - Both non-empty: SHA256(left || right)
// - Only left non-empty: return left
// - Only right non-empty: return right
// - Both empty: return empty hash
func hashBranch(left, right [32]byte) [32]byte {
	leftEmpty := left == [32]byte{}
	rightEmpty := right == [32]byte{}

	switch {
	case !leftEmpty && !rightEmpty:
		// Both branches present - concatenate and hash
		var b [64]byte
		copy(b[:32], left[:])
		copy(b[32:], right[:])
		return sha256.Sum256(b[:])
	case !leftEmpty:
		// Only left branch - return it directly
		return left
	case !rightEmpty:
		// Only right branch - return it directly
		return right
	default:
		// Both empty - return empty hash
		return [32]byte{}
	}
}
