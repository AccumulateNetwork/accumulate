// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The Jiuquan block-ledger migration (#4147): on the version transition, walk
// the root index chain and, for every past block, append a placeholder to the
// block-ledger log and delete the BPT entry of the old per-block account.
// Written and gated on both lines, never activated on mainnet — and never
// tested until now.

// seedPreJiuquanHistory writes `blocks` blocks of pre-Jiuquan history: a root
// index chain entry and a BlockLedger ACCOUNT per block, committed so every
// account has a BPT entry — the state mainnet is in today.
func seedPreJiuquanHistory(t *testing.T, db *database.Database, shim execute.DescribeShim, blocks int) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()

	acct := batch.Account(shim.Ledger())
	for i := 1; i <= blocks; i++ {
		_, err := addIndexChainEntry(acct.RootChain().Index(), &protocol.IndexEntry{
			Source:     uint64(i),
			BlockIndex: uint64(i),
		})
		require.NoError(t, err)

		bl := new(protocol.BlockLedger)
		bl.Url = shim.BlockLedger(uint64(i))
		bl.Index = uint64(i)
		bl.Time = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second)
		bl.Entries = []*protocol.BlockEntry{{Account: protocol.AccountUrl("alice"), Chain: "main", Index: uint64(i)}}
		require.NoError(t, batch.Account(bl.Url).Main().Put(bl))
	}
	// The real block flow gives every account its BPT entry at block end
	// (Close calls UpdateBPT); without this the accounts never enter the tree
	// and the migration has nothing to delete.
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
}

// jiuquanTransitionBlock returns a Block positioned exactly on the
// Vandenberg→Jiuquan transition, the way mainnet's activation block will be.
func jiuquanTransitionBlock(batch *database.Batch, shim execute.DescribeShim) *Block {
	x := new(Executor)
	x.Describe = shim
	x.globals = &Globals{
		Active:  core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Vandenberg},
		Pending: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Jiuquan},
	}
	return &Block{Batch: batch, Executor: x}
}

func bvn0Shim() execute.DescribeShim {
	return execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
}

// runMigration executes the post-update actions in their own batch and
// commits, mirroring how the real transition block commits its work.
func runMigration(t *testing.T, db *database.Database, shim execute.DescribeShim) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, jiuquanTransitionBlock(batch, shim).executePostUpdateActions())
	require.NoError(t, batch.Commit())
}

func TestBlockLedgerMigration_DeletesBptEntryForEveryPastBlock(t *testing.T) {
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)
	const blocks = 20
	seedPreJiuquanHistory(t, db, shim, blocks)

	// Precondition: the accounts hold BPT entries — that is the problem.
	batch := db.Begin(false)
	for i := 1; i <= blocks; i++ {
		_, err := batch.BPT().Get(batch.Account(shim.BlockLedger(uint64(i))).Key())
		require.NoError(t, err, "precondition: block %d must have a BPT entry before the migration", i)
	}
	batch.Discard()

	runMigration(t, db, shim)

	batch = db.Begin(false)
	defer batch.Discard()
	for i := 1; i <= blocks; i++ {
		_, err := batch.BPT().Get(batch.Account(shim.BlockLedger(uint64(i))).Key())
		assert.True(t, errors.Is(err, errors.NotFound),
			"block %d's ledger account must be out of the state tree after the migration", i)
	}
}

func TestBlockLedgerMigration_DoesNotDeleteTheAccountItself(t *testing.T) {
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)
	const blocks = 20
	seedPreJiuquanHistory(t, db, shim, blocks)
	runMigration(t, db, shim)

	batch := db.Begin(false)
	defer batch.Discard()
	for i := 1; i <= blocks; i++ {
		var bl *protocol.BlockLedger
		require.NoError(t, batch.Account(shim.BlockLedger(uint64(i))).Main().GetAs(&bl),
			"the migration removes only the BPT entry — the account must survive so historical queries keep working")
		assert.Equal(t, uint64(i), bl.Index)
	}
}

func TestBlockLedgerMigration_AppendsAPlaceholderPerHistoricalBlock(t *testing.T) {
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)
	const blocks = 20
	seedPreJiuquanHistory(t, db, shim, blocks)
	runMigration(t, db, shim)

	batch := db.Begin(false)
	defer batch.Discard()
	ledger := batch.Account(shim.Ledger())
	for i := 1; i <= blocks; i++ {
		_, entry, err := ledger.BlockLedger().Find(record.NewKey(uint64(i))).Exact().Get()
		require.NoError(t, err, "block %d must have a log entry", i)
		assert.Zero(t, entry.Index, "the placeholder is EMPTY — reads fall through to the account")
	}

	// And the fall-through works end to end: the empty placeholder yields the
	// old account's content.
	at, entries, err := indexing.LoadBlockLedger(ledger, 7)
	require.NoError(t, err)
	assert.False(t, at.IsZero())
	require.Len(t, entries, 1)
	assert.Equal(t, uint64(7), entries[0].Index)
}

// The 256-entry batching boundary: a chain whose height is not a multiple of
// 256 must be walked to the end, and the boundary crossing must not skip or
// duplicate an entry.
func TestBlockLedgerMigration_WalksTheWholeRootIndexChain(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds 300 blocks")
	}
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)
	const blocks = 300 // crosses 256, not a multiple of it
	seedPreJiuquanHistory(t, db, shim, blocks)
	runMigration(t, db, shim)

	batch := db.Begin(false)
	defer batch.Discard()
	ledger := batch.Account(shim.Ledger())
	for i := 1; i <= blocks; i++ {
		_, _, err := ledger.BlockLedger().Find(record.NewKey(uint64(i))).Exact().Get()
		require.NoError(t, err, "block %d fell off the walk — the 256-entry batching lost it", i)
		_, err = batch.BPT().Get(batch.Account(shim.BlockLedger(uint64(i))).Key())
		assert.True(t, errors.Is(err, errors.NotFound),
			"block %d's BPT entry survived — the 256-entry batching skipped it", i)
	}
}

func TestBlockLedgerMigration_EmptyChainMigratesCleanly(t *testing.T) {
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)

	// No history at all — the root index chain has never been written.
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, jiuquanTransitionBlock(batch, shim).executePostUpdateActions(),
		"a fresh partition must cross the transition without error")
}

// The plan asked for IsIdempotentIfRunTwice; the code is the authority, and
// the code is NOT idempotent: a second run fails on the first re-appended
// placeholder with "cannot index past entries" — the block-ledger log refuses
// keys at or below its last. That failure mode is acceptable only because a
// second run is unreachable: Active is updated after the transition block
// commits, and a restart reloads Active == Jiuquan from the database, which
// makes executePostUpdateActions a no-op. This pins both halves: the error is
// loud (not silent corruption), and the committed state survives intact.
func TestBlockLedgerMigration_SecondRunFailsLoudlyAndCorruptsNothing(t *testing.T) {
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)
	const blocks = 10
	seedPreJiuquanHistory(t, db, shim, blocks)

	runMigration(t, db, shim)

	// A replayed transition errors — it does not silently double-append.
	batch := db.Begin(true)
	err := jiuquanTransitionBlock(batch, shim).executePostUpdateActions()
	require.Error(t, err, "the log refuses to append past entries, so a re-run fails loudly")
	require.ErrorContains(t, err, "cannot index past entries")
	batch.Discard()

	// And the first run's committed state is untouched.
	batch = db.Begin(false)
	defer batch.Discard()
	ledger := batch.Account(shim.Ledger())
	for i := 1; i <= blocks; i++ {
		_, entry, err := ledger.BlockLedger().Find(record.NewKey(uint64(i))).Exact().Get()
		require.NoError(t, err)
		assert.Zero(t, entry.Index)
		var bl *protocol.BlockLedger
		require.NoError(t, batch.Account(shim.BlockLedger(uint64(i))).Main().GetAs(&bl),
			"the failed re-run must not have destroyed the account")
	}
}

// The migration fires only on the version TRANSITION — when Active == Pending
// there is nothing to do, and the walk must not run again on every block. A
// nil batch proves it: any touch would panic.
func TestBlockLedgerMigration_FiresOnlyOnTheVersionTransition(t *testing.T) {
	x := new(Executor)
	x.Describe = bvn0Shim()
	x.globals = &Globals{
		Active:  core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Jiuquan},
		Pending: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Jiuquan},
	}
	b := &Block{Batch: nil, Executor: x}

	require.NotPanics(t, func() {
		require.NoError(t, b.executePostUpdateActions())
	}, "with no transition pending, the migration must not touch the database at all")
}

// Characterization, not endorsement: the migration is keyed on the PENDING
// version being exactly Jiuquan. A transition that skips over Jiuquan —
// Vandenberg straight to Tanegashima or Kourou — does NOT run the walk, so
// the old BPT entries would stay forever. Mainnet must activate Jiuquan
// itself, not jump past it. This pins the constraint the activation plan
// (#4139, #4147) has to respect.
func TestBlockLedgerMigration_DoesNotFireWhenJiuquanIsSkippedOver(t *testing.T) {
	shim := bvn0Shim()
	db := database.OpenInMemory(nil)
	const blocks = 5
	seedPreJiuquanHistory(t, db, shim, blocks)

	batch := db.Begin(true)
	defer batch.Discard()
	x := new(Executor)
	x.Describe = shim
	x.globals = &Globals{
		Active:  core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Vandenberg},
		Pending: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Tanegashima},
	}
	require.NoError(t, (&Block{Batch: batch, Executor: x}).executePostUpdateActions())
	require.NoError(t, batch.Commit())

	batch = db.Begin(false)
	defer batch.Discard()
	for i := 1; i <= blocks; i++ {
		_, err := batch.BPT().Get(batch.Account(shim.BlockLedger(uint64(i))).Key())
		assert.NoError(t, err,
			"skipping over Jiuquan leaves block %d's BPT entry in place — the activation sequence must include Jiuquan itself", i)
	}
}
