// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// positionBlock builds a BVN0 block whose BVN1 synthetic stream stands at
// `delivered`/`received`, holding messages for the numbers listed.
func positionBlock(t *testing.T, delivered, received uint64, hold ...uint64) (*Block, stream) {
	t.Helper()
	x := streamTestExec(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = x.Describe.Synthetic()
	part := ledger.Partition(protocol.PartitionUrl("BVN1"))
	part.Delivered, part.Received = delivered, received
	part.Pending = make([]*url.TxID, received-delivered)
	for _, n := range hold {
		part.Pending[n-delivered-1] = ledger.Url.WithTxID([32]byte{byte(n)})
	}
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	return &Block{Batch: batch, Executor: x},
		stream{kind: streamSynthetic, ledger: ledger.Url, source: protocol.PartitionUrl("BVN1")}
}

func TestStreamPosition(t *testing.T) {
	// Delivered 10, received 14, holding 12 and 14 — so 11 and 13 are gaps we
	// know exist but cannot fill.
	b, s := positionBlock(t, 10, 14, 12, 14)
	p, err := b.positionOf(s)
	require.NoError(t, err)

	assert.Equal(t, uint64(11), p.next(), "the stream is waiting for delivered+1")

	assert.False(t, p.has(10), "already delivered, not staged")
	assert.False(t, p.has(11), "a gap: received past it, but we hold no message")
	assert.True(t, p.has(12))
	assert.False(t, p.has(13), "a gap")
	assert.True(t, p.has(14))
	assert.False(t, p.has(15), "beyond received")

	// The offset arithmetic is the part worth pinning: index = n-delivered-1.
	// Off by one here reads a neighbouring position, which is exactly the
	// failure a positional array invites.
	id, ok := p.idOf(12)
	require.True(t, ok)
	assert.Equal(t, [32]byte{12}, id.Hash())
	id, ok = p.idOf(14)
	require.True(t, ok)
	assert.Equal(t, [32]byte{14}, id.Hash())
}

// The point of the type: the ledger is read once per stream per block, however
// many times the position is asked for.
func TestStreamPosition_ReadsOncePerBlock(t *testing.T) {
	b, s := positionBlock(t, 0, 3, 2, 3)

	first, err := b.positionOf(s)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		again, err := b.positionOf(s)
		require.NoError(t, err)
		require.Same(t, first, again, "the position must be loaded once and shared, not re-read per caller")
	}
}

// Anchors and synthetics are separate streams even between the same pair of
// partitions, so they must not share a cache entry.
func TestStreamPosition_AnchorAndSyntheticAreDistinct(t *testing.T) {
	b, synth := positionBlock(t, 5, 5)

	anchorLedger := new(protocol.AnchorLedger)
	anchorLedger.Url = b.Executor.Describe.AnchorPool()
	anchorLedger.Anchor(protocol.PartitionUrl("BVN1")).Delivered = 99
	require.NoError(t, b.Batch.Account(anchorLedger.Url).Main().Put(anchorLedger))

	anchor := stream{kind: streamAnchor, ledger: anchorLedger.Url, source: protocol.PartitionUrl("BVN1")}

	ps, err := b.positionOf(synth)
	require.NoError(t, err)
	pa, err := b.positionOf(anchor)
	require.NoError(t, err)

	assert.Equal(t, uint64(6), ps.next())
	assert.Equal(t, uint64(100), pa.next(),
		"the anchor stream stands somewhere else entirely — sharing a position would let one gate the other")
}

// #4169 assumption 6.8, and the one implicated in the live drain failure: the
// drain loop clears b.positions between rounds so each round decides against
// the state the previous round left. If a cleared cache still reads the value
// from the start of the block, every round after the first decides against
// stale state — which looks exactly like the failure being chased, where
// rounds report progress while the ledger does not move.

func TestStreamPosition_ClearedCacheSeesACommittedAdvance(t *testing.T) {
	b, s := positionBlock(t, 0, 3, 2, 3)

	p1, err := b.positionOf(s)
	require.NoError(t, err)
	require.Equal(t, uint64(1), p1.next())

	// Advance the stream the way a delivery does: through a CHILD batch that
	// commits into the block's batch. That is the path executeRuns takes, and
	// it is the one that matters — a value written directly would prove less.
	child := b.Batch.Begin(true)
	var ledger *protocol.SyntheticLedger
	require.NoError(t, child.Account(s.ledger).Main().GetAs(&ledger))
	ledger.Partition(s.source).Add(true, 1, s.ledger.WithTxID([32]byte{1}))
	require.NoError(t, child.Account(s.ledger).Main().Put(ledger))
	require.NoError(t, child.Commit())

	// Still cached: within a round the position must not move under the code
	// deciding against it.
	p2, err := b.positionOf(s)
	require.NoError(t, err)
	require.Same(t, p1, p2, "within a round the position is fixed")
	require.Equal(t, uint64(1), p2.next())

	// Cleared: the next round must see what this block committed.
	b.positions = nil
	p3, err := b.positionOf(s)
	require.NoError(t, err)
	require.NotSame(t, p1, p3, "clearing must force a re-read")
	require.Equal(t, uint64(2), p3.next(),
		"a cleared cache must see the advance this block committed — otherwise every drain round after the first decides against stale state")
}

// The same, for the staged window rather than the watermark: a message
// recorded pending during the block must be visible to the next round, which
// is the whole reason the block re-drains.
func TestStreamPosition_ClearedCacheSeesAMessageRecordedThisBlock(t *testing.T) {
	b, s := positionBlock(t, 0, 0)

	p1, err := b.positionOf(s)
	require.NoError(t, err)
	require.False(t, p1.has(2), "nothing staged yet")

	child := b.Batch.Begin(true)
	var ledger *protocol.SyntheticLedger
	require.NoError(t, child.Account(s.ledger).Main().GetAs(&ledger))
	ledger.Partition(s.source).Add(false, 2, s.ledger.WithTxID([32]byte{2}))
	require.NoError(t, child.Account(s.ledger).Main().Put(ledger))
	require.NoError(t, child.Commit())

	b.positions = nil
	p2, err := b.positionOf(s)
	require.NoError(t, err)
	assert.True(t, p2.has(2),
		"a message recorded pending during the block must be drainable in a later round of the same block")
}
