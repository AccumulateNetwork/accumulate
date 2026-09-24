// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4412: a block whose only effect is an entry held behind a hole raises the
// stream's Received, and so is not empty. An empty block's batch is
// discarded, and the raise would go with it.
func TestReceived_AHoldOnlyBlockIsNotEmpty(t *testing.T) {
	b, s := positionBlock(t, 10)
	require.True(t, b.State.Empty(), "precondition")

	require.NoError(t, b.advanceStream(s, false, 13, txidFor(s, 13), nil))
	require.NoError(t, b.flushStreams())

	part := partitionOf(t, b, s)
	require.Equal(t, uint64(10), part.Delivered)
	require.Equal(t, uint64(13), part.Received)
	require.False(t, b.State.Empty(), "a block that raised Received must commit")

	// A block that raises nothing stays empty: a second flush of the same
	// block, or a later block holding a lower number, writes nothing.
	b2, _ := positionBlock(t, 10)
	b2.Batch = b.Batch
	require.NoError(t, b2.advanceStream(s, false, 12, txidFor(s, 12), nil))
	require.NoError(t, b2.flushStreams())
	require.Equal(t, uint64(13), partitionOf(t, b2, s).Received, "Received never decreases")
	require.True(t, b2.State.Empty(), "raising nothing is not a reason to commit")
}

// #4412: Received is hashed, so it is the block's count of what its own
// consensus messages carried, not a read of this node's staging. Two nodes
// whose staging differs -- one restarted and empty, one that already holds
// the entry and has validated a different hash at a higher number -- execute
// the same block and write the same Received.
func TestReceived_IsTheBlocksCountNotStagingsMemory(t *testing.T) {
	// The peer: its staging already holds 12 and 20 from earlier blocks.
	peer, s := positionBlock(t, 10, 12, 20)
	peer.staging.Commit()
	// The restarted node: its staging is empty.
	joiner, _ := positionBlock(t, 10)

	for _, b := range []*Block{peer, joiner} {
		require.NoError(t, b.advanceStream(s, false, 12, txidFor(s, 12), nil))
		require.NoError(t, b.advanceStream(s, false, 15, txidFor(s, 15), nil))
		require.NoError(t, b.flushStreams())
	}
	require.Equal(t, uint64(20), peer.staging.Sighted(s.id()), "precondition: the peer's staging has seen 20")
	require.Equal(t, uint64(15), joiner.staging.Sighted(s.id()), "precondition: the joiner's has not")

	require.Equal(t, uint64(15), partitionOf(t, peer, s).Received, "the peer's staging memory must not reach the ledger")
	require.Equal(t, partitionOf(t, peer, s).Received, partitionOf(t, joiner, s).Received)
}

// #4412: a number past the stage's span is not held anywhere, so it does not
// count; a number at or below Delivered is not an arrival. Neither writes
// anything, so Received stays what the fixture's ledger says (zero).
func TestReceived_CountsOnlyWhatStagingCanHold(t *testing.T) {
	b, s := positionBlock(t, 10)
	b.hold(s, 10, 10+execute.MaxStageSpan+1, &execute.Held{ID: txidFor(s, 1)})
	b.hold(s, 10, 9, &execute.Held{ID: txidFor(s, 9)})
	require.NoError(t, b.flushStreams())
	require.Zero(t, partitionOf(t, b, s).Received)
	require.True(t, b.State.Empty())
}

// #4412 review F4: a hold on a stream the block never positioned still
// reaches the ledger. Production positions every stream it has an arrival
// on before any message runs, so nothing reaches this today; the flush
// positions the stream rather than drop the mark, because a dropped mark is
// a wrong hashed value and not an error.
func TestReceived_AnUnpositionedHoldIsStillWritten(t *testing.T) {
	b, s := positionBlock(t, 10)
	b.hold(s, 10, 12, &execute.Held{ID: txidFor(s, 12)})
	require.NoError(t, b.flushStreams())
	require.Equal(t, uint64(12), partitionOf(t, b, s).Received)
	require.Equal(t, uint64(10), partitionOf(t, b, s).Delivered)
}

// #4412 review F8: positioning a stream reads its ledger and must not write
// it. A stream positioned by an arrival that then writes nothing (tossed,
// refused) must not leave an entry in the committed ledger.
func TestReceived_PositioningDoesNotWriteTheLedger(t *testing.T) {
	b, s := positionBlock(t, 10) // the ledger is Put, so dirty
	stranger := stream{kind: streamSynthetic, ledger: s.ledger, source: protocol.PartitionUrl("BVN7")}
	_, err := b.positionOf(stranger)
	require.NoError(t, err)

	var ledger *protocol.SyntheticLedger
	require.NoError(t, b.Batch.Account(s.ledger).Main().GetAs(&ledger))
	require.Len(t, ledger.Sequence, 1, "positioning inserted an entry for a stream that wrote nothing")
}
