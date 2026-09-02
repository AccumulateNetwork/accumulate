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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// positionBlock builds a BVN0 block whose BVN1 synthetic stream stands at
// `delivered`/`received`, holding messages for the numbers listed.
// positionBlock builds a block whose stream is delivered to `delivered` and
// holding `hold`.
//
// The two facts go to two different places, which is the change (#4189): the
// LEDGER carries Delivered, because what a block delivered is its output; the
// executor's STAGING carries what is held, because that is what the executor
// has taken in from consensus. Seeding the held set into the record is what the
// old fixture did, and it is what let the record and the database disagree.
func positionBlock(t *testing.T, delivered uint64, hold ...uint64) (*Block, stream) {
	t.Helper()
	x := streamTestExec(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = x.Describe.Synthetic()
	ledger.Partition(protocol.PartitionUrl("BVN1")).Delivered = delivered
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	s := stream{kind: streamSynthetic, ledger: ledger.Url, source: protocol.PartitionUrl("BVN1")}
	for _, n := range hold {
		x.Staging.Hold(s.id(), n, ledger.Url.WithTxID([32]byte{byte(n)}))
	}

	return &Block{positions: new(positionCache), Batch: batch, Executor: x}, s
}

func TestStreamPosition(t *testing.T) {
	// Delivered 10, received 14, holding 12 and 14 — so 11 and 13 are gaps we
	// know exist but cannot fill.
	b, s := positionBlock(t, 10, 12, 14)
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
	b, s := positionBlock(t, 0, 2, 3)

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
	b, synth := positionBlock(t, 5)

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
