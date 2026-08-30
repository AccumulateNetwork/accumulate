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
	return batch
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

// TestResolveBlockAtOrAfter proves resolution never moves backward: a height
// that is not itself indexed resolves to the next indexed block, never the
// previous one.
func TestResolveBlockAtOrAfter(t *testing.T) {
	// Indexed blocks 4, 9, 17 — gaps on both sides of 9
	batch := makeLedger(t, 4, 9, 17)

	cases := []struct {
		requested uint64
		resolved  uint64
	}{
		{4, 4},   // exactly the first
		{5, 9},   // in a gap, resolves forward
		{8, 9},   // just before an indexed block
		{9, 9},   // exactly on an indexed block
		{10, 17}, // in a gap, resolves forward
		{16, 17}, // just before the last
		{17, 17}, // exactly the last
	}
	for _, c := range cases {
		entry, err := ResolveBlockAtOrAfter(testPartition, batch, c.requested)
		require.NoErrorf(t, err, "requested %d", c.requested)
		require.Equalf(t, c.resolved, entry.BlockIndex, "requested %d", c.requested)
		require.GreaterOrEqualf(t, entry.BlockIndex, c.requested,
			"resolution moved backward for %d", c.requested)
	}
}

// TestResolveBlockAtOrAfter_BeforeGenesis proves a height below this node's
// horizon is refused rather than resolved forward to the earliest block it has.
func TestResolveBlockAtOrAfter_BeforeGenesis(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	for _, height := range []uint64{1, 2, 3} {
		_, err := ResolveBlockAtOrAfter(testPartition, batch, height)
		require.Errorf(t, err, "height %d", height)
		require.Equalf(t, errors.NotFound, errors.Code(err), "height %d", height)
		require.Containsf(t, err.Error(), "precedes this node's earliest indexed block 4", "height %d", height)
	}
}

// TestResolveBlockAtOrAfter_NotReached proves a height the partition has not
// reached is refused, and is refused distinguishably from a height below the
// horizon.
func TestResolveBlockAtOrAfter_NotReached(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	_, err := ResolveBlockAtOrAfter(testPartition, batch, 18)
	require.Error(t, err)
	require.Equal(t, errors.NotFound, errors.Code(err))
	require.Contains(t, err.Error(), "has not been reached")
	require.Contains(t, err.Error(), "latest indexed block is 17")
}

// TestResolveBlockAtOrAfter_Zero proves zero — which means the current state —
// is rejected rather than treated as a historical height.
func TestResolveBlockAtOrAfter_Zero(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	_, err := ResolveBlockAtOrAfter(testPartition, batch, 0)
	require.Error(t, err)
	require.Equal(t, errors.BadRequest, errors.Code(err))
	require.Contains(t, err.Error(), "zero means the current state")
}

func TestRetainedBlockRange(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)

	// Depth zero — the default, and every node today — retains nothing
	r, err := RetainedBlockRange(testPartition, batch, 0)
	require.NoError(t, err)
	require.True(t, r.IsEmpty())

	// A window shorter than the indexed range is clipped to the last depth blocks
	r, err = RetainedBlockRange(testPartition, batch, 5)
	require.NoError(t, err)
	require.Equal(t, BlockRange{Earliest: 13, Latest: 17}, r)

	// A window longer than the indexed range is clipped to what is indexed
	r, err = RetainedBlockRange(testPartition, batch, 1000)
	require.NoError(t, err)
	require.Equal(t, BlockRange{Earliest: 4, Latest: 17}, r)
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

	_, err := ResolveHistoricalAccountState(testPartition, batch, account, 9, 0)
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
	batch := makeLedger(t, 4, 9, 17)
	account := makeAccount(t, batch, protocol.AccountUrl("alice"), 4, 9, 17)

	// Requesting 5 resolves forward to 9, which the window covers
	entry, err := ResolveHistoricalAccountState(testPartition, batch, account, 5, 1000)
	require.NoError(t, err)
	require.Equal(t, uint64(9), entry.BlockIndex)
}

// TestResolveHistoricalAccountState_AccountAbsent proves an account that did
// not exist at the requested height is refused with NotFound, and is refused
// even though a later block would have proven it present.
func TestResolveHistoricalAccountState_AccountAbsent(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	// alice first appears at block 17
	account := makeAccount(t, batch, protocol.AccountUrl("alice"), 17)

	_, err := ResolveHistoricalAccountState(testPartition, batch, account, 9, 1000)
	require.Error(t, err)
	require.Equal(t, errors.NotFound, errors.Code(err))
	require.Contains(t, err.Error(), "did not exist at block 9")
	require.Contains(t, err.Error(), "earliest record of it is block 17")
}

// TestResolveHistoricalAccountState_Refusals asserts the three refusals are
// distinguishable by status code alone, which is what lets a client branch
// without parsing prose.
func TestResolveHistoricalAccountState_Refusals(t *testing.T) {
	batch := makeLedger(t, 4, 9, 17)
	absent := makeAccount(t, batch, protocol.AccountUrl("absent"), 17)
	present := makeAccount(t, batch, protocol.AccountUrl("present"), 4, 9, 17)

	cases := []struct {
		name    string
		account *database.Account
		height  uint64
		depth   uint64
		code    errors.Status
	}{
		{"before genesis", present, 2, 1000, errors.NotFound},
		{"not reached", present, 99, 1000, errors.NotFound},
		{"account absent", absent, 9, 1000, errors.NotFound},
		{"beyond retained range", present, 9, 0, errors.IncompleteChain},
		{"zero height", present, 0, 1000, errors.BadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ResolveHistoricalAccountState(testPartition, batch, c.account, c.height, c.depth)
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
		_, err := ResolveHistoricalAccountState(testPartition, batch, account, height, 0)
		require.Errorf(t, err, "height %d was answered", height)
	}
}
