// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"encoding/binary"
	"fmt"
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

const DefaultExecutorShardCount = 64

// ExecutorConfig holds executor configuration stored in the database.
type ExecutorConfig struct {
	ExecutorShardCount uint64
}

// ValidateShardCount checks that count is a power of 2 between 4 and 256.
func ValidateShardCount(count uint64) error {
	if count < 4 || count > 256 {
		return fmt.Errorf("shard count %d out of range [4, 256]", count)
	}
	if bits.OnesCount64(count) != 1 {
		return fmt.Errorf("shard count %d is not a power of 2", count)
	}
	return nil
}

// executorConfigKey is the key used to store the executor shard count.
var executorConfigKey = record.NewKey("ExecutorConfig", "ShardCount")

// getKVStore returns the underlying key-value store for the batch,
// walking up through parent batches to the root batch's changeset.
// Executor config bypasses batch model isolation intentionally since
// shard configuration is a global node setting, not transactional state.
func getKVStore(b *Batch) keyvalue.Store {
	for b.parent != nil {
		b = b.parent
	}
	return b.store.(interface{ Unwrap() keyvalue.Store }).Unwrap()
}

// GetExecutorConfig reads the executor configuration from the database.
// Returns a config with DefaultExecutorShardCount if no value has been stored.
func GetExecutorConfig(batch *Batch) (*ExecutorConfig, error) {
	data, err := getKVStore(batch).Get(executorConfigKey)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return &ExecutorConfig{ExecutorShardCount: DefaultExecutorShardCount}, nil
		}
		return nil, errors.UnknownError.Wrap(err)
	}
	if len(data) == 0 {
		return &ExecutorConfig{ExecutorShardCount: DefaultExecutorShardCount}, nil
	}
	v := binary.BigEndian.Uint64(data)
	if v == 0 {
		return &ExecutorConfig{ExecutorShardCount: DefaultExecutorShardCount}, nil
	}
	return &ExecutorConfig{ExecutorShardCount: v}, nil
}

// PutExecutorConfig validates and stores the executor configuration.
func PutExecutorConfig(batch *Batch, config *ExecutorConfig) error {
	if config == nil {
		return errors.BadRequest.With("config is nil")
	}
	if err := ValidateShardCount(config.ExecutorShardCount); err != nil {
		return errors.BadRequest.Wrap(err)
	}
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, config.ExecutorShardCount)
	return getKVStore(batch).Put(executorConfigKey, data)
}
