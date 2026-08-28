// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// BPTree is a minimal interface for BPT operations.
//
// NOTE: This interface is primarily used for testing and the factory pattern.
// In production code, prefer using concrete types (*BPT or *ShardedBPT) directly
// as they may have additional methods not in this interface.
//
// The interface allows the factory to return either implementation, but most
// code should use the concrete types for better type safety and access to
// implementation-specific methods.
type BPTree interface {
	// Insert adds or updates a key-value pair in the tree
	Insert(key *record.Key, value []byte) error

	// Get retrieves the value associated with the given key
	Get(key *record.Key) ([]byte, error)

	// Delete removes the entry for the given key
	Delete(key *record.Key) error

	// GetRootHash returns the root hash of the tree
	GetRootHash() ([32]byte, error)
}

// Ensure both implementations satisfy the interface
var _ BPTree = (*BPT)(nil)
var _ BPTree = (*ShardedBPT)(nil)

// Config contains configuration options for creating a BPT instance.
type Config struct {
	// ShardingEnabled determines whether to use ShardedBPT or regular BPT
	ShardingEnabled bool

	// ShardDepth specifies the number of bits for shard routing (1-8)
	// Only used if ShardingEnabled is true
	//
	// Work lands on shards by hash, so it does not spread evenly: with as many
	// shards as cores, collisions leave cores idle. Use more shards than cores
	// — two to four times — and expect diminishing returns past 64.
	//   - 5 = 32 shards
	//   - 6 = 64 shards
	ShardDepth int
}

// DefaultConfig returns a Config with sensible defaults:
// - Sharding disabled
// - Shard depth of 6 (64 shards) when enabled
//
// Sharding does not change the root, and this is not an empirical claim. The
// BPT routes on bits of the key hash, so its shape is a function of the key
// set; splitting at a bit boundary and recombining is the same hash
// computation reassociated. A sharded and an unsharded tree over the same data
// are necessarily identical, which is why enabling this is a configuration
// choice and not a protocol change. The equivalence tests exist to check the
// implementation honours that, not to establish it.
//
// It is off because ShardedBPT cannot yet serve production: it implements
// Insert, Get, Delete and GetRootHash, while the database also needs
// GetReceipt, Iterate and Index. Nothing outside tests calls this factory —
// internal/database builds a concrete *bpt.BPT directly. Turning this on
// without that work would change nothing; with it, this is the switch.
func DefaultConfig() Config {
	return Config{
		ShardingEnabled: false,
		ShardDepth:      6,
	}
}

// NewFromConfig creates a BPTree instance based on the provided configuration.
// If sharding is enabled, it returns a ShardedBPT; otherwise, it returns a regular BPT.
// This function provides a unified way to create BPT instances regardless of implementation.
//
// Example:
//
//	config := bpt.Config{
//	    ShardingEnabled: true,
//	    ShardDepth:      6,  // 64 shards
//	}
//	tree, err := bpt.NewFromConfig(config, store, key)
//	if err != nil {
//	    return err
//	}
//	// Use tree.Insert(), tree.Get(), etc. - works with either implementation
func NewFromConfig(config Config, store database.Store, key *record.Key) (BPTree, error) {
	if config.ShardingEnabled {
		return NewShardedBPT(store, key, config.ShardDepth)
	}
	return New(nil, nil, store, key), nil
}
