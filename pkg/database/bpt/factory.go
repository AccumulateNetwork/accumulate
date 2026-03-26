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
	// Recommended values:
	//   - 4 = 16 shards (optimal for 16-core systems)
	//   - 5 = 32 shards (for 32-core systems)
	//   - 6 = 64 shards (diminishing returns beyond this)
	ShardDepth int
}

// DefaultConfig returns a Config with sensible defaults:
// - Sharding disabled for backward compatibility
// - Shard depth of 4 (16 shards) when enabled
func DefaultConfig() Config {
	return Config{
		ShardingEnabled: false,
		ShardDepth:      4,
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
//	    ShardDepth:      4,  // 16 shards
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
