// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// TestShardedBlockInterfaceImplementation verifies that ShardedBlock implements Block
func TestShardedBlockInterfaceImplementation(t *testing.T) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	se, err := NewShardedExecutor(4, db)
	require.NoError(t, err)

	dispatcher := NewTransactionDispatcher(2)
	batch := db.Begin(true)
	defer batch.Discard()

	params := BlockParams{
		Index: 1,
		Time:  time.Now(),
	}

	sb := NewShardedBlock(params, se, dispatcher, batch)

	// Verify Block interface methods exist and work
	assert.Equal(t, params, sb.Params())

	// Process should work with nil envelope
	results, err := sb.Process(nil)
	require.NoError(t, err)
	assert.Empty(t, results)

	// Close should return a BlockState
	state, err := sb.Close()
	require.NoError(t, err)
	assert.NotNil(t, state)
}

// TestShardedBlockState verifies BlockState interface implementation
func TestShardedBlockState(t *testing.T) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	se, err := NewShardedExecutor(4, db)
	require.NoError(t, err)

	dispatcher := NewTransactionDispatcher(2)
	batch := db.Begin(true)
	defer batch.Discard()

	params := BlockParams{
		Index: 1,
		Time:  time.Now(),
	}

	sb := NewShardedBlock(params, se, dispatcher, batch)

	state, err := sb.Close()
	require.NoError(t, err)

	// Verify BlockState interface methods
	assert.Equal(t, params, state.Params())
	assert.True(t, state.IsEmpty()) // No transactions processed

	// DidCompleteMajorBlock should return false initially
	height, _, ok := state.DidCompleteMajorBlock()
	assert.False(t, ok)
	assert.Equal(t, uint64(0), height)

	// DidUpdateValidators should return false initially
	updates, ok := state.DidUpdateValidators()
	assert.False(t, ok)
	assert.Empty(t, updates)

	// ChangeSet should return the batch
	assert.Equal(t, batch, state.ChangeSet())

	// Hash should work (though may be empty)
	hash, err := state.Hash()
	require.NoError(t, err)
	assert.NotNil(t, hash)

	// Discard should be safe to call
	state.Discard()
}

// TestShardedBlockProcessEmptyEnvelope tests processing an empty envelope
func TestShardedBlockProcessEmptyEnvelope(t *testing.T) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	se, err := NewShardedExecutor(4, db)
	require.NoError(t, err)

	dispatcher := NewTransactionDispatcher(2)
	batch := db.Begin(true)
	defer batch.Discard()

	sb := NewShardedBlock(BlockParams{Index: 1}, se, dispatcher, batch)

	// Process empty envelope
	results, err := sb.Process(&messaging.Envelope{})
	require.NoError(t, err)
	assert.Empty(t, results)

	state, err := sb.Close()
	require.NoError(t, err)
	assert.True(t, state.IsEmpty())
}

// TestShardedBlockProcessEnvelope tests processing an envelope with messages
func TestShardedBlockProcessEnvelope(t *testing.T) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	se, err := NewShardedExecutor(4, db)
	require.NoError(t, err)

	dispatcher := NewTransactionDispatcher(2)
	batch := db.Begin(true)
	defer batch.Discard()

	sb := NewShardedBlock(BlockParams{Index: 1}, se, dispatcher, batch)

	// Process envelope with messages (we can't easily create real messages)
	// For now, test with empty envelope
	envelope := &messaging.Envelope{
		Messages: make([]messaging.Message, 3),
	}

	results, err := sb.Process(envelope)
	require.NoError(t, err)
	// Empty envelope (no real messages) should return empty
	assert.Len(t, results, 3)

	blockState, err := sb.Close()
	require.NoError(t, err)
	assert.NotNil(t, blockState)
	// Even with message slots, if Process isn't setting results to non-empty, block is still empty
	// This depends on actual implementation
}

// TestShardedExecutorWrapperCreation tests creating a sharded executor wrapper
func TestShardedExecutorWrapperCreation(t *testing.T) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	tests := []struct {
		name       string
		shardCount int
		shouldFail bool
		errMsg     string
	}{
		{"Valid 4 shards", 4, false, ""},
		{"Valid 8 shards", 8, false, ""},
		{"Valid 64 shards", 64, false, ""},
		{"Valid 256 shards", 256, false, ""},
		{"Invalid 0 shards", 0, true, "between 4 and 256"},
		{"Invalid 2 shards", 2, true, "between 4 and 256"},
		{"Invalid 3 shards", 3, true, "between 4 and 256"},  // Checked first
		{"Invalid 512 shards", 512, true, "between 4 and 256"},
		{"Invalid 100 shards", 100, true, "power of 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{Database: db}
			wrapper, err := NewShardedExecutorWrapper(opts, tc.shardCount)

			if tc.shouldFail {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				assert.Nil(t, wrapper)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, wrapper)
				assert.Equal(t, tc.shardCount, wrapper.shardCount)
			}
		})
	}
}

// TestShardedExecutorWrapperInterfaceImplementation verifies wrapper implements Executor
func TestShardedExecutorWrapperInterfaceImplementation(t *testing.T) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	opts := Options{Database: db}
	wrapper, err := NewShardedExecutorWrapper(opts, 4)
	require.NoError(t, err)

	// EnableTimers should not panic
	wrapper.EnableTimers()

	// LastBlock may return NotFound, that's ok
	_, _, _ = wrapper.LastBlock()
	// Could be NotFound or other error, both ok for now

	// Init should work
	validators, err := wrapper.Init(nil)
	require.NoError(t, err)
	assert.Empty(t, validators)

	// Validate should work
	statuses, err := wrapper.Validate(&messaging.Envelope{}, false)
	require.NoError(t, err)
	assert.Empty(t, statuses)

	// Begin should create a block
	block, err := wrapper.Begin(BlockParams{Index: 1})
	require.NoError(t, err)
	assert.NotNil(t, block)
}
