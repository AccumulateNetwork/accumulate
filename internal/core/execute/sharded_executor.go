// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"fmt"
	"math/bits"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ShardedExecutor routes accounts to shards and manages per-shard execution.
// Each account is deterministically assigned to a shard based on a hash of its
// URL. Shards execute independently with no cross-shard locking.
type ShardedExecutor struct {
	shardCount int
	shardDepth int // log2(shardCount), used for routing
	shards     []*PerShardExecutor
	db         database.Beginner
}

// NewShardedExecutor creates a ShardedExecutor with the given shard count.
// The shard count must be a power of two between 1 and 256.
func NewShardedExecutor(shardCount int, db database.Beginner) (*ShardedExecutor, error) {
	if shardCount < 1 || shardCount > 256 {
		return nil, fmt.Errorf("shard count must be between 1 and 256, got %d", shardCount)
	}
	if shardCount&(shardCount-1) != 0 {
		return nil, fmt.Errorf("shard count must be a power of two, got %d", shardCount)
	}

	depth := bits.TrailingZeros(uint(shardCount))
	se := &ShardedExecutor{
		shardCount: shardCount,
		shardDepth: depth,
		shards:     make([]*PerShardExecutor, shardCount),
		db:         db,
	}

	for i := range se.shards {
		se.shards[i] = newPerShardExecutor(i)
	}

	return se, nil
}

// ShardCount returns the number of shards.
func (se *ShardedExecutor) ShardCount() int {
	return se.shardCount
}

// RouteAccount returns the shard ID for the given account URL.
// Routing uses the ADI (identity) hash: IdentityAccountID32()[0] >> (8 - shardDepth).
// All accounts under the same ADI route to the same shard.
func (se *ShardedExecutor) RouteAccount(u *url.URL) int {
	if se.shardCount == 1 {
		return 0
	}
	h := u.IdentityAccountID32()
	return int(h[0] >> (8 - se.shardDepth))
}

// Shard returns the PerShardExecutor for the given shard ID.
func (se *ShardedExecutor) Shard(id int) *PerShardExecutor {
	return se.shards[id]
}

// ShardForAccount returns the PerShardExecutor responsible for the given account.
func (se *ShardedExecutor) ShardForAccount(u *url.URL) *PerShardExecutor {
	return se.shards[se.RouteAccount(u)]
}

// BeginBlock opens a new database batch on each shard, preparing them for
// transaction execution within a block. Must be called before executing
// transactions. Each shard gets its own independent writable batch.
func (se *ShardedExecutor) BeginBlock() {
	for _, s := range se.shards {
		s.BeginBatch(se.db)
	}
}

// Commit commits all shard batches. Shards are committed sequentially to the
// underlying database.
func (se *ShardedExecutor) Commit() error {
	for i, s := range se.shards {
		if err := s.Commit(); err != nil {
			return fmt.Errorf("shard %d commit: %w", i, err)
		}
	}
	return nil
}

// Discard discards all shard batches.
func (se *ShardedExecutor) Discard() {
	for _, s := range se.shards {
		s.Discard()
	}
}

// AccountsPerShard returns a mapping of shard ID to the subset of accounts
// routed to that shard. Useful for dispatching work.
func (se *ShardedExecutor) AccountsPerShard(accounts []*url.URL) map[int][]*url.URL {
	result := make(map[int][]*url.URL, se.shardCount)
	for _, u := range accounts {
		id := se.RouteAccount(u)
		result[id] = append(result[id], u)
	}
	return result
}

// ForEachShard calls fn for each shard in parallel. Returns the first error
// encountered, if any.
func (se *ShardedExecutor) ForEachShard(fn func(shard *PerShardExecutor) error) error {
	if se.shardCount == 1 {
		return fn(se.shards[0])
	}

	var wg sync.WaitGroup
	errs := make([]error, se.shardCount)
	for i, s := range se.shards {
		wg.Add(1)
		go func(idx int, shard *PerShardExecutor) {
			defer wg.Done()
			errs[idx] = fn(shard)
		}(i, s)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
