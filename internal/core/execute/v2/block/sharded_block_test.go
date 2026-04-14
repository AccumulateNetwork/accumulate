// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
)

func TestNewShardedBlock_InvalidShardCount(t *testing.T) {
	// Cannot create with non-power-of-two
	_, err := NewShardedBlock(nil, 3)
	require.Error(t, err)

	// Cannot create with 0
	_, err = NewShardedBlock(nil, 0)
	require.Error(t, err)

	// Cannot create with > 256
	_, err = NewShardedBlock(nil, 512)
	require.Error(t, err)
}

func TestNewShardedBlock_ValidShardCounts(t *testing.T) {
	for _, count := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256} {
		_, err := NewShardedBlock(nil, count)
		require.NoError(t, err, "shard count %d should be valid", count)
	}
}

func TestTransactionDispatcher_DispatchBlock(t *testing.T) {
	d := execute.NewTransactionDispatcher(6) // 64 shards
	require.Equal(t, 64, d.NumShards())
}

func TestShardedBlockMetrics(t *testing.T) {
	m := ShardedBlockMetrics{
		ShardCount:       64,
		MessagesPerShard: map[int]int{0: 10, 1: 5, 2: 15},
	}
	require.Equal(t, 64, m.ShardCount)
	require.Equal(t, 10, m.MessagesPerShard[0])
}

func TestNewShardedExecutorWrapper_InvalidShardCount(t *testing.T) {
	_, err := NewShardedExecutorWrapper(nil, 3)
	require.Error(t, err)

	_, err = NewShardedExecutorWrapper(nil, 0)
	require.Error(t, err)

	_, err = NewShardedExecutorWrapper(nil, 512)
	require.Error(t, err)
}
