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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4169 assumption 7.2, restated for #4189: advancing a stream once per block
// must leave exactly what advancing it once per message leaves.
//
// This is the one assumption here whose failure corrupts state rather than
// stalling a stream. A stream that stalls is visible and recoverable; a ledger
// whose Delivered disagrees with what actually executed is a divergent block
// hash, and every node that took the other path is now on a different chain.
//
// What the state IS has moved. The ledger carries Delivered, because what a
// block delivered is its output. What is HELD is the executor's staging, which
// no block writes. So the equivalence is over both — and the half that matters
// most is new: a receipt must leave the ledger record completely untouched. It
// used to grow a pending array there, which is what forced the array to be
// bounded, which is what livelocked the network.

func advTxid(n uint64) *url.TxID {
	return protocol.PartitionUrl("BVN0").WithTxID([32]byte{byte(n), byte(n >> 8)})
}

type advOp struct {
	number    uint64
	delivered bool
}

// applyOps runs a sequence of advances against a fresh block and returns what
// the two halves of the state say afterwards.
func applyOps(t *testing.T, delivered uint64, hold []uint64, ops []advOp) (*Block, stream) {
	t.Helper()
	b, s := positionBlock(t, delivered, hold...)
	for _, op := range ops {
		require.NoError(t, b.advanceStream(s, op.delivered, op.number, advTxid(op.number)))
	}
	return b, s
}

// heldNumbers reports which numbers the block is holding for a stream, in
// order: received, and above the delivery watermark. The record of a number
// survives its delivery — nothing is deleted, because Delivered is the cutoff —
// so the watermark is what makes this HELD rather than merely SEEN.
func heldNumbers(t *testing.T, b *Block, s stream, through uint64) []uint64 {
	t.Helper()
	var ledger protocol.SequenceLedger
	require.NoError(t, b.Batch.Account(s.ledger).Main().GetAs(&ledger))
	delivered := ledger.Partition(s.source).Delivered

	var held []uint64
	for n := delivered + 1; n <= through; n++ {
		_, ok, err := execute.IDOf(b.Batch, s.id(), n)
		require.NoError(t, err)
		if ok {
			held = append(held, n)
		}
	}
	return held
}

// seenNumbers is every number recorded, watermark or not.
func seenNumbers(t *testing.T, b *Block, s stream, through uint64) []uint64 {
	t.Helper()
	var seen []uint64
	for n := uint64(1); n <= through; n++ {
		_, ok, err := execute.IDOf(b.Batch, s.id(), n)
		require.NoError(t, err)
		if ok {
			seen = append(seen, n)
		}
	}
	return seen
}

// A run of consecutive deliveries — the case step 7 exists to collapse. One
// write, and it says the last number.
func TestStreamAdvance_ARunLeavesOneWatermark(t *testing.T) {
	for _, n := range []uint64{1, 2, 5, 40, 300} {
		ops := make([]advOp, 0, n)
		for i := uint64(1); i <= n; i++ {
			ops = append(ops, advOp{number: i, delivered: true})
		}
		b, s := applyOps(t, 0, nil, ops)
		require.NoError(t, b.flushStreams())
		assert.Equalf(t, n, partitionOf(t, b, s).Delivered, "run of %d", n)
	}
}

// A receipt is staging's alone. This is the property that removes the bound:
// nothing about holding a message reaches the record, so there is no array in
// the record to keep small.
func TestStreamAdvance_AReceiptDoesNotTouchTheLedger(t *testing.T) {
	b, s := applyOps(t, 0, nil, []advOp{{7, false}, {5, false}, {9, false}})
	require.NoError(t, b.flushStreams())

	part := partitionOf(t, b, s)
	assert.Equal(t, uint64(0), part.Delivered, "nothing was delivered")
	assert.Empty(t, part.Pending, "and nothing about what is HELD belongs in the record")
	assert.Equal(t, uint64(0), part.Received)

	assert.Equal(t, []uint64{5, 7, 9}, heldNumbers(t, b, s, 12), "staging has them")
	high, err := execute.Sighted(b.Batch, s.id())
	require.NoError(t, err)
	assert.Equal(t, uint64(9), high)
}

// A run followed by receipts: what a block does when it drains what it can and
// holds the rest.
func TestStreamAdvance_RunThenReceipts(t *testing.T) {
	ops := []advOp{
		{1, true}, {2, true}, {3, true}, // the run
		{7, false}, {5, false}, {9, false}, // arrivals past the gap, out of order
	}
	b, s := applyOps(t, 0, nil, ops)
	require.NoError(t, b.flushStreams())

	assert.Equal(t, uint64(3), partitionOf(t, b, s).Delivered)
	assert.Equal(t, []uint64{5, 7, 9}, heldNumbers(t, b, s, 12))
}

// Receipts interleaved with the run. If the outcome depends on the
// interleaving, a per-block advance CANNOT be substituted for a per-message
// one, and step 7 is unsound.
func TestStreamAdvance_InterleavingDoesNotMatter(t *testing.T) {
	interleaved := []advOp{
		{1, true}, {6, false}, {2, true}, {8, false}, {3, true}, {5, false},
	}
	grouped := []advOp{
		{1, true}, {2, true}, {3, true}, {6, false}, {8, false}, {5, false},
	}

	a, sa := applyOps(t, 0, nil, interleaved)
	require.NoError(t, a.flushStreams())
	c, sc := applyOps(t, 0, nil, grouped)
	require.NoError(t, c.flushStreams())

	assert.Equal(t, partitionOf(t, a, sa).Delivered, partitionOf(t, c, sc).Delivered,
		"the same operations in a different order")
	assert.Equal(t, heldNumbers(t, a, sa, 12), heldNumbers(t, c, sc, 12))
}

// Draining an existing held window, which is what a backlog presents. The
// release happens on COMMIT, so the drain leaves staging untouched until then.
func TestStreamAdvance_DrainingAnExistingWindow(t *testing.T) {
	ops := make([]advOp, 0, 5)
	for i := uint64(11); i <= 15; i++ {
		ops = append(ops, advOp{number: i, delivered: true})
	}
	b, s := applyOps(t, 10, []uint64{11, 12, 13, 14, 15}, ops)
	require.NoError(t, b.flushStreams())

	assert.Equal(t, uint64(15), partitionOf(t, b, s).Delivered)
	assert.Empty(t, heldNumbers(t, b, s, 20),
		"a fully drained window holds nothing: every number is at or below the watermark")

	// Nothing was deleted to achieve that. Delivered is the cutoff, so the
	// records stay and simply stop counting as held — which is why there is no
	// release step to get the timing of wrong.
	assert.Equal(t, []uint64{11, 12, 13, 14, 15}, seenNumbers(t, b, s, 20))
	high, err := execute.Sighted(b.Batch, s.id())
	require.NoError(t, err)
	assert.Equal(t, uint64(15), high,
		"the high-water mark stays: the stream WAS behind, and forgetting that says it never was")
}
