// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// PerShardExecutor manages execution state for a single shard. Each shard
// operates on an independent database batch and maintains its own account
// cache. No cross-shard locking is needed because accounts are
// deterministically assigned to shards and never move.
type PerShardExecutor struct {
	ID    int
	mu    sync.Mutex
	batch *database.Batch

	// loadedAccounts tracks which accounts have been accessed in this shard
	// during the current block, for diagnostics and testing.
	loadedAccounts map[[32]byte]struct{}
}

func newPerShardExecutor(id int) *PerShardExecutor {
	return &PerShardExecutor{
		ID:             id,
		loadedAccounts: make(map[[32]byte]struct{}),
	}
}

// BeginBatch opens a new writable database batch for this shard.
func (ps *PerShardExecutor) BeginBatch(db database.Beginner) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.batch = db.Begin(true)
	ps.loadedAccounts = make(map[[32]byte]struct{})
}

// Batch returns the shard's current database batch.
func (ps *PerShardExecutor) Batch() *database.Batch {
	return ps.batch
}

// Account returns the database account handle for the given URL within this
// shard's batch. It also records the account as loaded.
func (ps *PerShardExecutor) Account(u *url.URL) *database.Account {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.loadedAccounts[u.AccountID32()] = struct{}{}
	return ps.batch.Account(u)
}

// LoadedAccountCount returns the number of unique accounts accessed in this
// shard during the current block.
func (ps *PerShardExecutor) LoadedAccountCount() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.loadedAccounts)
}

// Commit commits this shard's database batch.
func (ps *PerShardExecutor) Commit() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.batch == nil {
		return nil
	}
	err := ps.batch.Commit()
	ps.batch = nil
	return err
}

// Discard discards this shard's database batch without committing.
func (ps *PerShardExecutor) Discard() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.batch != nil {
		ps.batch.Discard()
		ps.batch = nil
	}
}
