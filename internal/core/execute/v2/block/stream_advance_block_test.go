// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4169 step 7: staging owns the ledger write. A stream's position is the
// block's WORKING COPY of its ledger entry — read once, advanced in place as
// the block executes, and written back once when the block closes. These pin
// the three things that have to be true for that to be sound.

func txidFor(s stream, n uint64) *url.TxID {
	return s.ledger.WithTxID([32]byte{byte(n), byte(n >> 8)})
}

func partitionOf(t *testing.T, b *Block, s stream) *protocol.PartitionSyntheticLedger {
	t.Helper()
	var ledger protocol.SequenceLedger
	require.NoError(t, b.Batch.Account(s.ledger).Main().GetAs(&ledger))
	return ledger.Partition(s.source)
}

// An advance is visible to the next ask within the block — this is what the
// drain rounds (and a package whose members are consecutive) depend on. It
// used to be true because every message wrote the ledger and the cache was
// cleared between rounds; now it is true because the position IS the state.
func TestStreamAdvance_IsVisibleWithinTheBlock(t *testing.T) {
	b, s := positionBlock(t, 0, 2, 3)

	p, err := b.positionOf(s)
	require.NoError(t, err)
	require.Equal(t, uint64(1), p.next())

	require.NoError(t, b.advanceStream(s, true, 1, txidFor(s, 1)))
	again, err := b.positionOf(s)
	require.NoError(t, err)
	require.Same(t, p, again, "the position is advanced, not replaced")
	assert.Equal(t, uint64(2), p.next(), "delivering 1 makes 2 next")
	assert.True(t, p.has(2), "the staged tail is still there")

	require.NoError(t, b.advanceStream(s, true, 2, txidFor(s, 2)))
	require.NoError(t, b.advanceStream(s, true, 3, txidFor(s, 3)))
	assert.Equal(t, uint64(4), p.next())
	assert.False(t, p.has(3), "delivered, so no longer staged")

	// And the ledger itself has NOT moved: the write is the block's, at close.
	assert.Equal(t, uint64(0), partitionOf(t, b, s).Delivered,
		"the ledger record is written once per block, not per message")
}

// The same for a receipt: a message recorded pending during the block is
// drainable in a later round of the same block.
func TestStreamAdvance_ARecordedMessageIsVisibleWithinTheBlock(t *testing.T) {
	b, s := positionBlock(t, 0)

	p, err := b.positionOf(s)
	require.NoError(t, err)
	require.False(t, p.has(2))

	require.NoError(t, b.advanceStream(s, false, 2, txidFor(s, 2)))
	assert.True(t, p.has(2))
	assert.False(t, p.has(1), "1 is a gap: known received, not held")
	assert.Equal(t, uint64(2), p.received())
}

// The proof step 7 owes, restated for #4189: closing the block leaves the
// watermark the deliveries reached, once, over a mix of deliveries and
// out-of-order receipts — and leaves nothing else.
func TestStreamAdvance_FlushWritesTheWatermarkAndNothingElse(t *testing.T) {
	ops := []advOp{
		{1, true}, {6, false}, {2, true}, {8, false}, {3, true}, {5, false}, {4, true}, {5, true},
	}

	b, s := positionBlock(t, 0, 2, 3)
	for _, op := range ops {
		require.NoError(t, b.advanceStream(s, op.delivered, op.number, txidFor(s, op.number)))
	}
	require.NoError(t, b.flushStreams())

	part := partitionOf(t, b, s)
	require.Equal(t, uint64(5), part.Delivered, "1..5 delivered, 2 and 3 from the staged tail")
	require.Empty(t, part.Pending, "the held set is staging's, and staging is not written")
	require.Equal(t, uint64(0), part.Received, "nor is the high-water mark")

	// Staging still holds everything, including what was just delivered: the
	// block has not committed, so the delivery has not happened.
	require.Equal(t, []uint64{2, 3, 5, 6, 8}, heldNumbers(b, s, 12))

	// On commit, what was delivered goes and what arrived past the gap stays.
	b.releaseStreams()
	require.Equal(t, []uint64{6, 8}, heldNumbers(b, s, 12))

	// Flushing is idempotent per block: a second flush writes the same thing.
	require.NoError(t, b.flushStreams())
	require.Equal(t, uint64(5), partitionOf(t, b, s).Delivered, "a second flush changes nothing")
}

// The flush must not clobber what something else wrote to the same record
// during the block. produceSynthetic bumps Produced on the same ledger at
// close; the flush is a read-modify-write, not a put of the working copy.
func TestStreamAdvance_FlushPreservesOtherWritesToTheRecord(t *testing.T) {
	b, s := positionBlock(t, 0)
	require.NoError(t, b.advanceStream(s, false, 2, txidFor(s, 2)))

	var ledger *protocol.SyntheticLedger
	require.NoError(t, b.Batch.Account(s.ledger).Main().GetAs(&ledger))
	ledger.Partition(s.source).Produced = 77
	require.NoError(t, b.Batch.Account(s.ledger).Main().Put(ledger))

	require.NoError(t, b.advanceStream(s, true, 1, txidFor(s, 1)))
	require.NoError(t, b.flushStreams())
	part := partitionOf(t, b, s)
	assert.Equal(t, uint64(77), part.Produced, "a write made between load and flush must survive")
	assert.Equal(t, uint64(1), part.Delivered, "and the flush still writes its own field")
}

// The livelock, inverted (#4189). A receipt far past the delivery point is
// HELD, not refused.
//
// It used to be refused past MaxPendingSequenced — 4,096 — because the held set
// was an array in a record hashed into the BPT every block, and an array in
// such a record cannot grow without limit. But refusing the receipt did not
// refuse the message: the body was stored anyway, so the cap discarded only the
// INDEX of what the node held. The node then held a message and reported not
// holding it, and healing fetched it back across the partition, forever. In one
// twenty-minute soak that was 8,556 sequence numbers fetched 53,011 times, with
// every partition live the whole time.
//
// Staging is not hashed and not written, so there is nothing left to bound.
func TestStreamAdvance_HoldsAFarFutureReceipt(t *testing.T) {
	b, s := positionBlock(t, 10)

	for _, far := range []uint64{10 + 4096, 10 + 4097, 10 + 100_000} {
		require.NoError(t, b.advanceStream(s, false, far, txidFor(s, far)))

		p, err := b.positionOf(s)
		require.NoError(t, err)
		assert.Truef(t, p.has(far), "%d must be held, however far ahead it is", far)
		assert.Equalf(t, far, p.received(), "and counted as seen")
	}

	// And none of it reached the record.
	require.NoError(t, b.flushStreams())
	part := partitionOf(t, b, s)
	assert.Equal(t, uint64(10), part.Delivered)
	assert.Empty(t, part.Pending)
}

// Step 7 is the only step that can corrupt state: a wrong advance moves a
// watermark that does not self-correct. Both ways of being wrong are refused
// loudly rather than applied.
func TestStreamAdvance_RefusesAnOutOfOrderAdvance(t *testing.T) {
	b, s := positionBlock(t, 10, 11, 12)

	err := b.advanceStream(s, false, 10, txidFor(s, 10))
	require.Error(t, err, "recording as pending something already delivered")

	err = b.advanceStream(s, true, 12, txidFor(s, 12))
	require.Error(t, err, "delivering 12 while 11 is next skips a message")

	err = b.advanceStream(s, true, 10, txidFor(s, 10))
	require.Error(t, err, "re-delivering 10 would shift the pending window under 11")

	p, err := b.positionOf(s)
	require.NoError(t, err)
	assert.Equal(t, uint64(11), p.next(), "nothing moved")
	assert.True(t, p.has(11))
	assert.True(t, p.has(12))
}

// The quadratic dies here. The per-message cost of advancing a stream must not
// scale with the stream's backlog: the copy is paid once per block, not once
// per message. This is the cost test moved from step 2, where it could not be
// had (the read had not moved yet).
func TestStreamAdvance_CostDoesNotScaleWithBacklog(t *testing.T) {
	perMessage := func(backlog uint64) uint64 {
		const N = 100
		b, s := positionBlock(t, 0)
		part := partitionOf(t, b, s)
		for i := range part.Pending {
			part.Pending[i] = txidFor(s, uint64(i+1))
		}
		var ledger *protocol.SyntheticLedger
		require.NoError(t, b.Batch.Account(s.ledger).Main().GetAs(&ledger))
		require.NoError(t, b.Batch.Account(s.ledger).Main().Put(ledger))

		// The once-per-block read is outside the measurement on purpose: it
		// is the cost being divided across the run, and the assertion is
		// about the per-message increment.
		_, err := b.positionOf(s)
		require.NoError(t, err)

		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for i := uint64(1); i <= N; i++ {
			require.NoError(t, b.advanceStream(s, true, i, txidFor(s, i)))
		}
		runtime.ReadMemStats(&after)
		return (after.TotalAlloc - before.TotalAlloc) / N
	}

	small := perMessage(100)
	large := perMessage(16000)
	t.Logf("advance: backlog 100 → %d B/msg, backlog 16000 → %d B/msg", small, large)
	require.Less(t, large, small*3+256,
		"the per-message cost of an advance must not scale with the backlog (#4164, #4169 step 7)")
}
