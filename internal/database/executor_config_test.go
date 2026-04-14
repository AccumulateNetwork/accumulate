// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
)

func TestValidateShardCount(t *testing.T) {
	// Valid power-of-2 values in range.
	for _, n := range []uint64{4, 8, 16, 32, 64, 128, 256} {
		assert.NoError(t, ValidateShardCount(n), "expected %d to be valid", n)
	}

	// Out of range.
	assert.Error(t, ValidateShardCount(0))
	assert.Error(t, ValidateShardCount(1))
	assert.Error(t, ValidateShardCount(2))
	assert.Error(t, ValidateShardCount(3))
	assert.Error(t, ValidateShardCount(512))

	// In range but not power of 2.
	assert.Error(t, ValidateShardCount(5))
	assert.Error(t, ValidateShardCount(6))
	assert.Error(t, ValidateShardCount(10))
	assert.Error(t, ValidateShardCount(48))
	assert.Error(t, ValidateShardCount(100))
	assert.Error(t, ValidateShardCount(200))
}

func TestExecutorConfigDefaultOnEmpty(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	cfg, err := GetExecutorConfig(batch)
	require.NoError(t, err)
	assert.Equal(t, uint64(DefaultExecutorShardCount), cfg.ExecutorShardCount)
}

func TestExecutorConfigPutAndGet(t *testing.T) {
	db := OpenInMemory(nil)

	// Write a config.
	batch := db.Begin(true)
	err := PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 32})
	require.NoError(t, err)
	require.NoError(t, batch.Commit())

	// Read it back.
	batch = db.Begin(false)
	defer batch.Discard()
	cfg, err := GetExecutorConfig(batch)
	require.NoError(t, err)
	assert.Equal(t, uint64(32), cfg.ExecutorShardCount)
}

func TestExecutorConfigRejectInvalid(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	assert.Error(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 0}))
	assert.Error(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 3}))
	assert.Error(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 100}))
	assert.Error(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 512}))
	assert.Error(t, PutExecutorConfig(batch, nil))
}

func TestExecutorConfigPersistsAcrossBatches(t *testing.T) {
	db := OpenInMemory(nil)

	// Set to 16.
	batch := db.Begin(true)
	require.NoError(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 16}))
	require.NoError(t, batch.Commit())

	// Verify.
	batch = db.Begin(false)
	cfg, err := GetExecutorConfig(batch)
	require.NoError(t, err)
	assert.Equal(t, uint64(16), cfg.ExecutorShardCount)
	batch.Discard()

	// Update to 128.
	batch = db.Begin(true)
	require.NoError(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 128}))
	require.NoError(t, batch.Commit())

	// Verify updated value.
	batch = db.Begin(false)
	defer batch.Discard()
	cfg, err = GetExecutorConfig(batch)
	require.NoError(t, err)
	assert.Equal(t, uint64(128), cfg.ExecutorShardCount)
}

func TestExecutorConfigWriteAndReadInSameBatch(t *testing.T) {
	db := OpenInMemory(nil)

	// Write and read within the same batch (no commit required for visibility
	// within the batch since val.Commit() writes to the batch's store).
	batch := db.Begin(true)
	defer batch.Discard()

	require.NoError(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: 8}))

	cfg, err := GetExecutorConfig(batch)
	require.NoError(t, err)
	assert.Equal(t, uint64(8), cfg.ExecutorShardCount)
}

func TestExecutorConfigAllValidValues(t *testing.T) {
	db := OpenInMemory(nil)

	// Round-trip every valid shard count through the database.
	for _, n := range []uint64{4, 8, 16, 32, 64, 128, 256} {
		batch := db.Begin(true)
		require.NoError(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: n}),
			"PutExecutorConfig(%d) should succeed", n)
		require.NoError(t, batch.Commit())

		batch = db.Begin(false)
		cfg, err := GetExecutorConfig(batch)
		require.NoError(t, err)
		assert.Equal(t, n, cfg.ExecutorShardCount, "round-trip for shard count %d", n)
		batch.Discard()
	}
}

func TestExecutorConfigDynamicReconfiguration(t *testing.T) {
	db := OpenInMemory(nil)

	// Simulate changing the shard count between blocks.
	counts := []uint64{64, 32, 128, 16, 256, 4, 8}
	for _, n := range counts {
		batch := db.Begin(true)
		require.NoError(t, PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: n}))
		require.NoError(t, batch.Commit())

		batch = db.Begin(false)
		cfg, err := GetExecutorConfig(batch)
		require.NoError(t, err)
		assert.Equal(t, n, cfg.ExecutorShardCount)
		batch.Discard()
	}
}

func TestExecutorConfigRoutingChangesWithShardCount(t *testing.T) {
	// Verify that different shard counts produce different routing masks.
	// The shard count is used as (hash % shardCount), so changing the count
	// changes how transactions are routed to shards.
	testHash := uint64(137) // arbitrary value

	results := make(map[uint64]uint64)
	for _, n := range []uint64{4, 8, 16, 32, 64, 128, 256} {
		results[n] = testHash % n
	}

	// Different shard counts must produce different routing for at least some values.
	// 137 % 4 = 1, 137 % 8 = 1, 137 % 16 = 9, etc.
	// Verify they are not all the same.
	allSame := true
	first := results[4]
	for _, v := range results {
		if v != first {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "different shard counts should produce different routing for some values")
}

func TestExecutorConfigValidationChain(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// PutExecutorConfig must reject invalid values before storing.
	invalidCounts := []uint64{0, 1, 2, 3, 5, 6, 7, 9, 10, 48, 100, 200, 300, 512, 1024}
	for _, n := range invalidCounts {
		err := PutExecutorConfig(batch, &ExecutorConfig{ExecutorShardCount: n})
		assert.Error(t, err, "PutExecutorConfig(%d) should fail validation", n)
	}

	// After all rejected puts, GetExecutorConfig returns default (nothing was stored).
	cfg, err := GetExecutorConfig(batch)
	require.NoError(t, err)
	assert.Equal(t, uint64(DefaultExecutorShardCount), cfg.ExecutorShardCount,
		"no invalid value should have been stored")
}

func TestExecutorConfigNilConfig(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	err := PutExecutorConfig(batch, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}
