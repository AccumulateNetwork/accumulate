// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify defaults
	require.False(t, cfg.ShardingEnabled, "sharding should be disabled by default")
	require.Equal(t, 4, cfg.ShardDepth, "default shard depth should be 4")
}

func TestNewFromConfig_NonSharded(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")
	config := Config{
		ShardingEnabled: false,
		ShardDepth:      4,
	}

	tree, err := NewFromConfig(config, store, key)
	require.NoError(t, err)

	// Verify it's a regular BPT
	_, ok := tree.(*BPT)
	require.True(t, ok, "expected *BPT, got %T", tree)
}

func TestNewFromConfig_Sharded(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")
	config := Config{
		ShardingEnabled: true,
		ShardDepth:      4,
	}

	tree, err := NewFromConfig(config, store, key)
	require.NoError(t, err)

	// Verify it's a ShardedBPT
	sharded, ok := tree.(*ShardedBPT)
	require.True(t, ok, "expected *ShardedBPT, got %T", tree)
	require.Equal(t, 4, sharded.shardDepth)
	require.Equal(t, 16, sharded.numShards)
}

func TestNewFromConfig_InvalidDepth(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")

	testCases := []struct {
		name  string
		depth int
	}{
		{"depth too low", 0},
		{"depth too high", 9},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				ShardingEnabled: true,
				ShardDepth:      tc.depth,
			}

			_, err := NewFromConfig(config, store, key)
			require.Error(t, err, "should fail for invalid depth")
		})
	}
}

func TestBPTreeInterface_NonSharded(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")
	config := Config{ShardingEnabled: false}

	tree, err := NewFromConfig(config, store, key)
	require.NoError(t, err)

	// Test interface methods
	testKey := record.NewKey("account", "test1")
	testValue := sha256.Sum256([]byte("value1"))

	err = tree.Insert(testKey, testValue[:])
	require.NoError(t, err)

	retrievedValue, err := tree.Get(testKey)
	require.NoError(t, err)
	require.Equal(t, testValue[:], retrievedValue)

	rootHash, err := tree.GetRootHash()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, rootHash)

	err = tree.Delete(testKey)
	require.NoError(t, err)
}

func TestBPTreeInterface_Sharded(t *testing.T) {
	kvs := memory.New(nil).Begin(nil, true)
	defer kvs.Discard()
	store := keyvalue.RecordStore{Store: kvs}

	key := record.NewKey("test")
	config := Config{
		ShardingEnabled: true,
		ShardDepth:      4,
	}

	tree, err := NewFromConfig(config, store, key)
	require.NoError(t, err)

	// Test interface methods
	testKey := record.NewKey("account", "test1")
	testValue := sha256.Sum256([]byte("value1"))

	err = tree.Insert(testKey, testValue[:])
	require.NoError(t, err)

	retrievedValue, err := tree.Get(testKey)
	require.NoError(t, err)
	require.Equal(t, testValue[:], retrievedValue)

	rootHash, err := tree.GetRootHash()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, rootHash)

	err = tree.Delete(testKey)
	require.NoError(t, err)
}

func TestBPTreeInterface_SameRootHash(t *testing.T) {
	// Verify that sharded and non-sharded BPTs produce the same root hash
	// for the same data
	kvs1 := memory.New(nil).Begin(nil, true)
	defer kvs1.Discard()
	store1 := keyvalue.RecordStore{Store: kvs1}

	kvs2 := memory.New(nil).Begin(nil, true)
	defer kvs2.Discard()
	store2 := keyvalue.RecordStore{Store: kvs2}

	key := record.NewKey("test")

	// Create non-sharded BPT
	tree1, err := NewFromConfig(Config{ShardingEnabled: false}, store1, key)
	require.NoError(t, err)

	// Create sharded BPT
	tree2, err := NewFromConfig(Config{ShardingEnabled: true, ShardDepth: 4}, store2, key)
	require.NoError(t, err)

	// Insert same data into both
	testData := []struct {
		key   *record.Key
		value [32]byte
	}{
		{record.NewKey("account", "test1"), sha256.Sum256([]byte("value1"))},
		{record.NewKey("account", "test2"), sha256.Sum256([]byte("value2"))},
		{record.NewKey("account", "test3"), sha256.Sum256([]byte("value3"))},
		{record.NewKey("account", "test4"), sha256.Sum256([]byte("value4"))},
	}

	for _, td := range testData {
		require.NoError(t, tree1.Insert(td.key, td.value[:]))
		require.NoError(t, tree2.Insert(td.key, td.value[:]))
	}

	// Get root hashes
	hash1, err := tree1.GetRootHash()
	require.NoError(t, err)

	hash2, err := tree2.GetRootHash()
	require.NoError(t, err)

	// Should be identical
	require.Equal(t, hash1, hash2, "sharded and non-sharded BPTs should produce identical root hashes")
}
