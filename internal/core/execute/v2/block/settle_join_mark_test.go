// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// THE SETTLE IS WHAT MARKS THE JOIN, AND IT IS THE ONLY THING THAT DOES
// (#4295, test audit gap 3).
//
// Cache.JoinedAt is what makes a node answer "not from me" instead of
// NotFound for the blocks it did not execute, and this line — the last line
// of SettleStaging — is its only production caller. Nothing exercised it: the
// sequencer's tests set the mark by hand, so the entire chain from "the join
// settled at Q" to "the sequencer refuses for Q" rested on one unexecuted
// statement.
//
// It also asserts the BOUND, which is the half that keeps a miss visible: the
// counts handed to the cache are the ones the PULLED STATE carries — what
// each stream and the anchor sequence had reached at Q — so a number above
// them is this node's own and its absence is a miss (threat review finding
// 2).
//
// Nothing here is by hand but the ledger values: the executor is the real
// one, the settle is `SettleStaging`, and the cache is the one
// `x.synthCache()` hands the block.
func TestSettleStagingMarksTheJoinWithWhatWasProduced(t *testing.T) {
	const q = 3

	s := newStagingSim(t, 6)
	j := newJoiner(t, s)

	// A block collected but not executed, so the settle has something to do.
	env := s.packageEnvelope(0, 2, 3)
	s.packageArrives(0, 2, 3)
	j.collect(t, 1, env)
	s.newBlock()

	// The pull brings the joining node's store to the peer's state.
	j.pull(t)

	// What that state says the streams had reached. Written explicitly so the
	// numbers are legible; the point of the test is that the settle READS
	// them from the store and hands them to the cache, not where they came
	// from.
	dst := protocol.PartitionUrl("BVN1")
	synth := j.x.Describe.Synthetic()
	batch := j.db.Begin(true)
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synth
	ledger.Partition(dst).Produced = 7
	require.NoError(t, batch.Account(synth).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	// The anchor sequence chain's height is the anchor count (block_begin.go,
	// recordAnchor). Two anchors produced by Q.
	batch = j.db.Begin(true)
	chain, err := batch.Account(j.x.Describe.AnchorPool()).AnchorSequenceChain().Get()
	require.NoError(t, err)
	require.NoError(t, chain.AddEntry(make([]byte, 32), false))
	require.NoError(t, chain.AddEntry(make([]byte, 32), false))
	require.NoError(t, batch.Commit())

	cache := j.x.synthCache()
	require.Zero(t, cache.Joined(), "the cache is not marked before the settle")

	j.settle(t, q)

	// The mark, from the only production caller there is.
	require.Equal(t, uint64(q), cache.Joined(),
		"the settle did not tell the cache which block this node joined at; "+
			"without it the sequencer answers NotFound for blocks the node never executed (#4294)")

	// And the bound the pulled state carried.
	joined, notMine := cache.NotMine(dst, 7)
	require.True(t, notMine, "the last number produced before the join is not this node's")
	require.Equal(t, uint64(q), joined, "the refusal must name the block it joined at")

	_, notMine = cache.NotMine(dst, 8)
	require.False(t, notMine,
		"a number produced AFTER the join was treated as 'not from me'; "+
			"that is what makes a real miss invisible on every node that ever restarted")

	joined, notMine = cache.AnchorNotMine(2)
	require.True(t, notMine, "the last anchor produced before the join is not this node's")
	require.Equal(t, uint64(q), joined)
	_, notMine = cache.AnchorNotMine(3)
	require.False(t, notMine, "an anchor produced after the join was treated as 'not from me'")

	// A stream this partition produced nothing for before the join has a
	// bound of zero, so every number for it is this node's own.
	_, notMine = cache.NotMine(protocol.PartitionUrl("BVN9"), 1)
	require.False(t, notMine, "a stream with nothing produced before the join bounded nothing")
}
