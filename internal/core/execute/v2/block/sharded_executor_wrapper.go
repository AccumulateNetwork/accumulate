// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ShardedExecutorWrapper wraps a v2 Executor to produce ShardedBlocks instead
// of regular Blocks. All methods except Begin delegate directly to the inner
// executor. Begin creates a normal Block and then wraps it with ShardedBlock.
type ShardedExecutorWrapper struct {
	inner      *Executor
	shardCount int
	logger     logging.OptionalLogger
}

// NewShardedExecutorWrapper creates a wrapper that produces sharded blocks.
// The shard count must be a power of two between 1 and 256.
func NewShardedExecutorWrapper(inner *Executor, shardCount int) (*ShardedExecutorWrapper, error) {
	// Validate shard count early (same checks as NewShardedBlock)
	if shardCount < 1 || shardCount > 256 {
		return nil, fmt.Errorf("shard count must be between 1 and 256, got %d", shardCount)
	}
	if shardCount&(shardCount-1) != 0 {
		return nil, fmt.Errorf("shard count must be a power of two, got %d", shardCount)
	}

	w := &ShardedExecutorWrapper{
		inner:      inner,
		shardCount: shardCount,
	}
	if inner != nil && inner.logger.L != nil {
		w.logger.Set(inner.logger.L, "module", "sharded-executor")
	}
	w.logger.Info("Sharded executor wrapper created", "shard-count", shardCount)
	return w, nil
}

func (w *ShardedExecutorWrapper) EnableTimers() {
	w.inner.EnableTimers()
}

func (w *ShardedExecutorWrapper) StoreBlockTimers(ds *logging.DataSet) {
	w.inner.StoreBlockTimers(ds)
}

func (w *ShardedExecutorWrapper) LastBlock() (*execute.BlockParams, [32]byte, error) {
	return w.inner.LastBlock()
}

func (w *ShardedExecutorWrapper) Init(validators []*execute.ValidatorUpdate) (additional []*execute.ValidatorUpdate, err error) {
	return w.inner.Init(validators)
}

func (w *ShardedExecutorWrapper) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
	return w.inner.Validate(envelope, recheck)
}

// Begin creates a block using the inner executor, then wraps it with
// ShardedBlock for parallel message processing.
func (w *ShardedExecutorWrapper) Begin(params execute.BlockParams) (execute.Block, error) {
	block, err := w.inner.Begin(params)
	if err != nil {
		return nil, err
	}

	innerBlock, ok := block.(*Block)
	if !ok {
		// If for some reason the inner executor doesn't return *Block, fall back
		w.logger.Error("Block execution is SEQUENTIAL", "reason", "inner executor returned unexpected block type", "type", fmt.Sprintf("%T", block))
		return block, nil
	}

	sb, err := NewShardedBlock(innerBlock, w.shardCount)
	if err != nil {
		// If sharding fails, fall back to sequential
		w.logger.Error("Block execution is SEQUENTIAL", "reason", "sharded block creation failed", "error", err, "shard-count", w.shardCount)
		return block, nil
	}

	w.logger.Debug("Block execution is SHARDED", "shard-count", w.shardCount)
	return sb, nil
}
