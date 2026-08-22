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

// paddedMutex is a mutex padded to a cache line to prevent false sharing.
// On most architectures, cache lines are 64 bytes. A sync.Mutex is 8 bytes,
// so without padding, 8 mutexes would share a cache line, causing severe
// performance degradation from false sharing.
type paddedMutex struct {
	mu sync.Mutex
	_  [56]byte // Padding to 64 bytes (64 - 8 = 56)
}

// ShardedBPT is a BPT that partitions the tree at a configurable depth into
// independent shards for parallel updates. Each shard is a standard BPT
// instance with its own locking, providing embarrassingly parallel operations
// with zero contention between shards.
//
// Thread Safety:
//   - Each shard's BPT is accessed only under that shard's lock
//   - Different keys route to different shards with zero contention
//   - Same key always routes to same shard, protected by that shard's lock
//   - The base BPT type does not require external synchronization because
//     each instance is accessed by only one shard under that shard's lock
//
// The sharding is based on the natural binary structure of the tree. At depth
// N, the tree has 2^N branches, and each branch becomes an independent shard.
// Keys are routed to shards using the high-order bits of the key hash.
//
// Storage format is identical to non-sharded BPT - no database changes needed.
// The tree structure itself provides natural partitioning.
type ShardedBPT struct {
	shardDepth int           // Number of bits for routing (4, 5, or 6)
	numShards  int           // Number of shards (2^shardDepth)
	shards     []*BPT        // Array of standard BPT instances
	shardMu    []paddedMutex // Per-shard locks (padded to prevent false sharing)
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
// The baseKey parameter is the storage location prefix for all shard data.
// Each shard will store its data under baseKey/shard-N where N is the shard index.
// Sharding does not change the BPT. At depth 6 the nodes six levels into the
// tree ARE the shards: a shard is the subtree under one level-6 node. Entries
// under distinct level-6 nodes occupy disjoint key prefixes, so they cannot
// conflict and can be resolved in parallel without locking — that is the whole
// point. Resolve the 64 subtrees, then resolve levels 0-5 from their roots.
// Sharding changes where a search starts, not what is stored.
//
// This implementation deviates: it gives each shard its own BPT under its own
// record key (BPT/shard-0, BPT/shard-1, ...), so nodes land at different
// record keys than an unsharded tree would use. The roots still agree — a
// shard's keys share their leading bits, so its own upper levels collapse
// through hashBranch and its root equals the level-6 subtree hash — but the
// bytes on disk differ, which they should not.
//
// That is an implementation defect, not a property of sharding. Receipts, for
// instance, are perfectly possible either way: the sibling hashes above a
// shard are derivable from the 64 shard roots, which are always in hand. They
// are simply not implemented here.
//
// The fix is small and is specified in full in #4135: a shard root is
// addressable as nodeKeyAt(depth, prefix) in the one tree, so ShardedBPT
// should hold one *BPT and treat a shard as a cursor rather than a tree.
func NewShardedBPT(store database.Store, key *record.Key, depth int) (*ShardedBPT, error) {
	if depth < 1 || depth > 8 {
		return nil, errors.BadRequest.WithFormat("shard depth must be between 1 and 8, got %d", depth)
	}

	numShards := 1 << depth
	s := &ShardedBPT{
		shardDepth: depth,
		numShards:  numShards,
		shards:     make([]*BPT, numShards),
		shardMu:    make([]paddedMutex, numShards),
		store:      store,
		key:        key,
	}

	// Create a BPT instance for each shard
	// Each shard gets its own key prefix to avoid storage collisions
	for i := 0; i < numShards; i++ {
		shardStorageKey := key.Append(fmt.Sprintf("shard-%d", i))
		s.shards[i] = New(nil, nil, store, shardStorageKey)
	}

	return s, nil
}

// routeToShard determines which shard a key belongs to based on the high-order
// bits of the key hash. This uses the same routing logic as the BPT's internal
// tree structure. Returns both the shard and its index for locking.
//
// Routing algorithm:
//
//	For depth=4: extract bits 7-4 from first byte (0b11110000 >> 4 = shard 0-15)
//	For depth=5: extract bits 7-3 from first byte (0b11111000 >> 3 = shard 0-31)
//	For depth=6: extract bits 7-2 from first byte (0b11111100 >> 2 = shard 0-63)
//
// The shift amount (8 - depth) moves the high-order bits to the low positions.
func (s *ShardedBPT) routeToShard(keyHash [32]byte) (int, *BPT) {
	const bitsPerByte = 8
	// Extract high-order bits for shard routing
	shardID := int(keyHash[0] >> (bitsPerByte - s.shardDepth))
	return shardID, s.shards[shardID]
}

// Insert updates or inserts a value for the given key. The operation is
// routed to the appropriate shard based on the key hash. Thread-safe with
// per-shard locking.
func (s *ShardedBPT) Insert(key *record.Key, value []byte) error {
	shardID, shard := s.routeToShard(key.Hash())
	s.shardMu[shardID].mu.Lock()
	defer s.shardMu[shardID].mu.Unlock()
	return shard.Insert(key, value)
}

// Get retrieves the value associated with the given key. The operation is
// routed to the appropriate shard based on the key hash. Thread-safe with
// per-shard locking.
func (s *ShardedBPT) Get(key *record.Key) ([]byte, error) {
	shardID, shard := s.routeToShard(key.Hash())
	s.shardMu[shardID].mu.Lock()
	defer s.shardMu[shardID].mu.Unlock()
	return shard.Get(key)
}

// Delete removes the entry for the given key, if present. The operation is
// routed to the appropriate shard based on the key hash. Thread-safe with
// per-shard locking.
func (s *ShardedBPT) Delete(key *record.Key) error {
	shardID, shard := s.routeToShard(key.Hash())
	s.shardMu[shardID].mu.Lock()
	defer s.shardMu[shardID].mu.Unlock()
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

			s.shardMu[idx].mu.Lock()
			defer s.shardMu[idx].mu.Unlock()

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

	// Check for errors - collect all to avoid losing diagnostic info
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		// Return first error (could also use errors.Join in Go 1.20+)
		return [32]byte{}, errs[0]
	}

	// Combine the shard roots hierarchically
	return s.combineShardRoots(shardRoots), nil
}

// combineShardRoots combines an array of shard root hashes into a single
// root hash by building a virtual binary tree bottom-up.
//
// CRITICAL CORRECTNESS REQUIREMENT:
// BPT routing uses inverted bits where bit=1 goes LEFT and bit=0 goes RIGHT.
// This is opposite to typical binary tree conventions!
//
// Therefore, when combining adjacent shards:
//   - Shard i (even, bit=0) represents the RIGHT branch
//   - Shard i+1 (odd, bit=1) represents the LEFT branch
//   - Combined hash must be: hash(LEFT, RIGHT) = hash(i+1, i)
//
// This inversion is essential for producing identical root hashes to
// non-sharded BPT. Any change to this logic will break correctness.
//
// The algorithm pairs adjacent roots and hashes them together, repeating
// until only one root remains. For odd numbers of roots, the last root
// is carried forward to the next level.
func (s *ShardedBPT) combineShardRoots(roots [][32]byte) [32]byte {
	current := roots

	// Build virtual tree bottom-up
	for len(current) > 1 {
		next := make([][32]byte, (len(current)+1)/2)
		for i := 0; i < len(current); i += 2 {
			if i+1 < len(current) {
				// CRITICAL: Hash (odd, even) not (even, odd) due to bit inversion
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
