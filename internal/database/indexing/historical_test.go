// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// testPartition is the partition the historical tests resolve against.
var testPartition = config.NetworkUrl{URL: protocol.PartitionUrl("BVN0")}

// appendIndexEntries appends the given entries to an index chain.
func appendIndexEntries(t testing.TB, c *database.Chain2, entries ...*protocol.IndexEntry) {
	t.Helper()
	for _, entry := range entries {
		b, err := entry.MarshalBinary()
		require.NoError(t, err)
		require.NoError(t, c.Inner().AddEntry(b, false))
	}
}

// makeLedger builds a partition ledger whose root index chain records one entry
// per listed block. Blocks are listed explicitly because a real ledger indexes
// only the blocks that produced a root chain entry, so the sequence has gaps —
// which is precisely what at-or-after resolution exists to handle.
func makeLedger(t testing.TB, blocks ...uint64) *database.Batch {
	t.Helper()
	batch, _ := makeLedgerDB(t, blocks...)
	return batch
}

func makeLedgerDB(t testing.TB, blocks ...uint64) (*database.Batch, *database.Database) {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	rootIndex := batch.Account(testPartition.Ledger()).RootChain().Index()
	for i, block := range blocks {
		appendIndexEntries(t, rootIndex, &protocol.IndexEntry{
			Source:     uint64(10 * i),
			BlockIndex: block,
		})
	}
	return batch, db
}

// retainFrom makes the ledger actually retain BPT history from the given
// height, by superseding a node with retention enabled — the same path a block
// takes. The retained range is read from what was retained, never from a
// configured depth, so a test cannot fake it by passing a number.
func retainFrom(t testing.TB, db *database.Database, height, depth uint64) {
	t.Helper()
	key := record.NewKey("aip58", "probe")

	b1 := db.Begin(true)
	defer b1.Discard()
	var v1, v2 [32]byte
	v1[0], v2[0] = 1, 2
	require.NoError(t, b1.BPT().Insert(key, v1[:]))
	require.NoError(t, b1.Commit())

	b2 := db.Begin(true)
	defer b2.Discard()
	b2.SetBPTHistory(height, depth)
	require.NoError(t, b2.BPT().Insert(key, v2[:]))
	require.NoError(t, b2.Commit())
}

// makeAccount records the account as having been updated in the given blocks,
// by appending to its main chain index.
func makeAccount(t testing.TB, batch *database.Batch, account *url.URL, blocks ...uint64) *database.Account {
	t.Helper()
	rec := batch.Account(account)
	mainIndex := rec.MainChain().Index()
	for i, block := range blocks {
		appendIndexEntries(t, mainIndex, &protocol.IndexEntry{
			Source:     uint64(i),
			BlockIndex: block,
		})
	}
	return rec
}

func TestBlockRange(t *testing.T) {
	empty := BlockRange{Earliest: 1, Latest: 0}
	require.True(t, empty.IsEmpty())
	require.False(t, empty.Contains(0))
	require.False(t, empty.Contains(1))
	require.Equal(t, "empty", empty.String())

	r := BlockRange{Earliest: 5, Latest: 9}
	require.False(t, r.IsEmpty())
	require.False(t, r.Contains(4))
	require.True(t, r.Contains(5))
	require.True(t, r.Contains(7))
	require.True(t, r.Contains(9))
	require.False(t, r.Contains(10))
	require.Equal(t, "[5, 9]", r.String())

	// A single-block range is not empty
	one := BlockRange{Earliest: 3, Latest: 3}
	require.False(t, one.IsEmpty())
	require.True(t, one.Contains(3))
}

func TestIndexedBlockRange(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	r, err := IndexedBlockRange(testPartition, batch)
	require.NoError(t, err)
	require.Equal(t, BlockRange{Earliest: 4, Latest: 17}, r)
}

func TestIndexedBlockRange_Empty(t *testing.T) {
	batch := makeLedger(t)
	_, err := IndexedBlockRange(testPartition, batch)
	require.Error(t, err)
	require.Equal(t, errors.InternalError, errors.Code(err))
}

// TestResolveBlockAtOrBefore proves resolution never moves forward: a height
// that is not itself indexed resolves to the previous indexed block, never the
// next one.
//
// This is the direction that makes the answer exact. An unindexed block changed
// no state, so its BPT root is the previous indexed block's root. Resolving
// forward would return state that had not happened yet at the requested height.
func TestResolveBlockAtOrBefore(t *testing.T) {
	// Indexed blocks 4, 9, 17 — gaps on both sides of 9
	batch := makeLedger(t, 4, 9, 17)

	cases := []struct {
		requested uint64
		resolved  uint64
		pos       uint64
	}{
		{4, 4, 0},   // exactly the first
		{5, 4, 0},   // in a gap, resolves back
		{8, 4, 0},   // just before an indexed block
		{9, 9, 1},   // exactly on an indexed block
		{10, 9, 1},  // in a gap, resolves back
		{16, 9, 1},  // just before the last
		{17, 17, 2}, // exactly the last
	}
	for _, c := range cases {
		pos, entry, err := ResolveBlockAtOrBefore(testPartition, batch, c.requested)
		require.NoErrorf(t, err, "requested %d", c.requested)
		require.Equalf(t, c.resolved, entry.BlockIndex, "requested %d", c.requested)
		require.Equalf(t, c.pos, pos, "position for %d", c.requested)
		require.LessOrEqualf(t, entry.BlockIndex, c.requested,
			"resolution moved forward for %d", c.requested)
	}
}

// TestResolveBlockAtOrBefore_BeforeGenesis proves a height below this node's
// horizon is refused rather than resolved forward to the earliest block it has.
func TestResolveBlockAtOrBefore_BeforeGenesis(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	for _, height := range []uint64{1, 2, 3} {
		_, _, err := ResolveBlockAtOrBefore(testPartition, batch, height)
		require.Errorf(t, err, "height %d", height)
		require.Equalf(t, errors.NotFound, errors.Code(err), "height %d", height)
		require.Containsf(t, err.Error(), "precedes this node's earliest indexed block 4", "height %d", height)
	}
}

// TestResolveBlockAtOrBefore_NotReached proves a height beyond what the node
// has indexed is refused, and is refused distinguishably from a height below
// the horizon. It is NOT resolved back to the latest indexed block: the node
// cannot tell a recent empty block from one that has not happened.
func TestResolveBlockAtOrBefore_NotReached(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	_, _, err := ResolveBlockAtOrBefore(testPartition, batch, 18)
	require.Error(t, err)
	require.Equal(t, errors.NotFound, errors.Code(err))
	require.Contains(t, err.Error(), "is beyond this node's latest indexed block")
	require.Contains(t, err.Error(), "latest indexed block 17")
}

// TestResolveBlockAtOrBefore_Zero proves zero — which means the current state —
// is rejected rather than treated as a historical height.
func TestResolveBlockAtOrBefore_Zero(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	_, _, err := ResolveBlockAtOrBefore(testPartition, batch, 0)
	require.Error(t, err)
	require.Equal(t, errors.BadRequest, errors.Code(err))
	require.Contains(t, err.Error(), "zero means the current state")
}

func TestRetainedBlockRange(t *testing.T) {
	// A node that has retained nothing — the default, and every node today —
	// advertises an empty range
	batch, db := makeLedgerDB(t, 4, 9, 17)
	r, err := RetainedBlockRange(testPartition, batch)
	require.NoError(t, err)
	require.True(t, r.IsEmpty())
	require.NoError(t, batch.Commit())

	// Once it has actually retained, the range starts at what it kept, not at
	// what it was configured to keep
	retainFrom(t, db, 14, 5) // floor = 14-5 = 9
	batch2 := db.Begin(false)
	t.Cleanup(batch2.Discard)
	r, err = RetainedBlockRange(testPartition, batch2)
	require.NoError(t, err)
	require.Equal(t, BlockRange{Earliest: 14, Latest: 17}, r,
		"the first retaining block is the horizon; nothing before it was kept")
}

// TestRetainedBlockRange_ClippedToIndexed proves the advertised range never
// claims a block the ledger has not indexed.
func TestRetainedBlockRange_ClippedToIndexed(t *testing.T) {
	batch, db := makeLedgerDB(t, 40, 90, 170)
	require.NoError(t, batch.Commit())
	retainFrom(t, db, 2, 1000)
	batch2 := db.Begin(false)
	t.Cleanup(batch2.Discard)

	r, err := RetainedBlockRange(testPartition, batch2)
	require.NoError(t, err)
	require.Equal(t, BlockRange{Earliest: 40, Latest: 170}, r)
}

func TestAccountFirstIndexedBlock(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	account := makeAccount(t, batch, protocol.AccountUrl("alice"), 9, 17)

	block, ok, err := AccountFirstIndexedBlock(account)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(9), block)
}

// TestAccountFirstIndexedBlock_Unknown proves an account with no indexed main
// chain reports "cannot tell" rather than "block zero", so a caller cannot turn
// silence into a claim that the account was absent.
func TestAccountFirstIndexedBlock_Unknown(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	account := makeAccount(t, batch, protocol.AccountUrl("alice"))

	block, ok, err := AccountFirstIndexedBlock(account)
	require.NoError(t, err)
	require.False(t, ok)
	require.Zero(t, block)
}

// TestResolveHistoricalAccountState_NotRetained is the behaviour of every node
// today: the height resolves, the account existed, and the node still refuses —
// with IncompleteChain, naming its empty retained range — because it keeps no
// BPT history. This is the refusal that must never become a current-state
// receipt.
func TestResolveHistoricalAccountState_NotRetained(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	account := makeAccount(t, batch, protocol.AccountUrl("alice"), 4, 9, 17)

	_, err := ResolveHistoricalAccountState(testPartition, batch, account, 9)
	require.Error(t, err)
	require.Equal(t, errors.IncompleteChain, errors.Code(err))
	require.Contains(t, err.Error(), "no BPT history retained for block 9")
	require.Contains(t, err.Error(), "retained range is empty")
}

// TestResolveHistoricalAccountState_Retained proves the resolution path itself
// is sound: given a depth that covers the height, the resolved entry is
// returned rather than a refusal. Nothing sets a non-zero depth today; this
// exercises the machinery Phase 4 will supply state for.
func TestResolveHistoricalAccountState_Retained(t *testing.T) {
	batch, db := makeLedgerDB(t, 4, 9, 17)
	makeAccount(t, batch, protocol.AccountUrl("alice"), 4, 9, 17)
	require.NoError(t, batch.Commit())
	retainFrom(t, db, 4, 1000)

	b := db.Begin(false)
	t.Cleanup(b.Discard)

	// Requesting 5 resolves back to 4, which the retained window covers
	entry, err := ResolveHistoricalAccountState(testPartition, b, b.Account(protocol.AccountUrl("alice")), 5)
	require.NoError(t, err)
	require.Equal(t, uint64(4), entry.BlockIndex)
}

// TestResolveHistoricalAccountState_AccountAbsent proves an account that did
// not exist at the requested height is refused with NotFound, and is refused
// even though a later block would have proven it present.
func TestResolveHistoricalAccountState_AccountAbsent(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	// alice first appears at block 17
	account := makeAccount(t, batch, protocol.AccountUrl("alice"), 17)

	_, err := ResolveHistoricalAccountState(testPartition, batch, account, 9)
	require.Error(t, err)
	require.Equal(t, errors.NotFound, errors.Code(err))
	require.Contains(t, err.Error(), "did not exist at block 9")
	require.Contains(t, err.Error(), "earliest record of it is block 17")
}

// TestResolveHistoricalAccountState_Refusals asserts the three refusals are
// distinguishable by status code alone, which is what lets a client branch
// without parsing prose.
func TestResolveHistoricalAccountState_Refusals(t *testing.T) {
	batch, db := makeLedgerDB(t, 4, 9, 17)
	makeAccount(t, batch, protocol.AccountUrl("absent"), 17)
	makeAccount(t, batch, protocol.AccountUrl("present"), 4, 9, 17)
	require.NoError(t, batch.Commit())

	// Retain from block 4, so the retained-range refusal is out of the way for
	// every case except the one that tests it
	retainFrom(t, db, 4, 1000)

	cases := []struct {
		name    string
		account string
		height  uint64
		retain  bool
		code    errors.Status
	}{
		{"before genesis", "present", 2, true, errors.NotFound},
		{"not reached", "present", 99, true, errors.NotFound},
		{"account absent", "absent", 9, true, errors.NotFound},
		{"beyond retained range", "present", 9, false, errors.IncompleteChain},
		{"zero height", "present", 0, true, errors.BadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The no-retention case needs a ledger that retained nothing, which
			// is a different database — retention cannot be un-done
			b := db.Begin(false)
			defer b.Discard()
			if !c.retain {
				b2, _ := makeLedgerDB(t, 4, 9, 17)
				makeAccount(t, b2, protocol.AccountUrl(c.account), 4, 9, 17)
				b = b2
			}
			_, err := ResolveHistoricalAccountState(testPartition, b, b.Account(protocol.AccountUrl(c.account)), c.height)
			require.Error(t, err)
			require.Equal(t, c.code, errors.Code(err))
		})
	}
}

// TestResolveHistoricalAccountState_NeverCurrent is the rule that matters most:
// with retention disabled, no requested height — including the current one —
// produces an answer. A node that cannot prove the past must say so, not hand
// back the present.
func TestResolveHistoricalAccountState_NeverCurrent(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	account := makeAccount(t, batch, protocol.AccountUrl("alice"), 4, 9, 17)

	for height := uint64(1); height <= 20; height++ {
		_, err := ResolveHistoricalAccountState(testPartition, batch, account, height)
		require.Errorf(t, err, "height %d was answered", height)
	}
}
