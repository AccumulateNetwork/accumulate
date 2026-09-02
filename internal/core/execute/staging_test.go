// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func stream(t *testing.T) execute.StreamID {
	t.Helper()
	return execute.StreamID{
		Ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic),
		Source: protocol.PartitionUrl("BVN1"),
	}
}

func txid(n uint64) *url.TxID {
	return protocol.PartitionUrl("BVN1").WithTxID([32]byte{byte(n), byte(n >> 8)})
}

// seedLedger puts the stream's ledger account so the BPT has an entry for it.
func seedLedger(t *testing.T, db *database.Database, id execute.StreamID) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = id.Ledger
	ledger.Partition(id.Source).Delivered = 0
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.UpdateBPT()) // so the account is IN the BPT before anything measures it
	require.NoError(t, batch.Commit())
}

// Staging decides what a block executes: a block delivers the contiguous run
// from Delivered+1 taken from this block's arrivals AND from what is already
// held. So a node that came back holding less than its peers would execute a
// shorter run and produce a different block hash. It has to survive.
func TestStaging_SurvivesACommit(t *testing.T) {
	db := database.OpenInMemory(nil)
	id := stream(t)
	seedLedger(t, db, id)

	batch := db.Begin(true)
	require.NoError(t, execute.Hold(batch, id, 7, txid(7)))
	require.NoError(t, execute.Hold(batch, id, 9, txid(9)))
	require.NoError(t, batch.Commit())

	// A different batch entirely — as a restarted node would open.
	batch = db.Begin(false)
	defer batch.Discard()

	got, ok, err := execute.IDOf(batch, id, 7)
	require.NoError(t, err)
	require.True(t, ok, "a held message must survive the commit")
	assert.Equal(t, txid(7).String(), got.String())

	high, err := execute.Sighted(batch, id)
	require.NoError(t, err)
	assert.Equal(t, uint64(9), high)

	runs, err := execute.Missing(batch, id, 0, high, 8)
	require.NoError(t, err)
	assert.Equal(t, [][2]uint64{{1, 6}, {8, 8}}, runs, "and so must the gaps it implies")
}

// A discarded block holds nothing. Staging rides the block's batch, so "the
// node holds it" and "the node says it holds it" are the same statement — which
// is what removes the need for a release step whose timing could be got wrong.
func TestStaging_IsDiscardedWithItsBlock(t *testing.T) {
	db := database.OpenInMemory(nil)
	id := stream(t)
	seedLedger(t, db, id)

	batch := db.Begin(true)
	require.NoError(t, execute.Hold(batch, id, 7, txid(7)))
	batch.Discard()

	batch = db.Begin(false)
	defer batch.Discard()
	_, ok, err := execute.IDOf(batch, id, 7)
	require.NoError(t, err)
	assert.False(t, ok, "a receipt recorded by a block that never happened was never received")
}

// THE property that removes the bound. Staging is durable but NOT hashed: it is
// a deterministic function of the consensus stream, so every node derives the
// same set from the same input and it does not need to be hashed to be agreed.
//
// Hashing it is what forced it into an account's main state, and main state is
// rewritten whole every block, which is what forced MaxPendingSequenced — and
// past that bound the executor stored a message while refusing to record that
// it had it. If this test fails, the bound has to come back.
func TestStaging_IsNotHashed(t *testing.T) {
	db := database.OpenInMemory(nil)
	id := stream(t)
	seedLedger(t, db, id)

	root := func() [32]byte {
		batch := db.Begin(true)
		defer batch.Discard()
		require.NoError(t, batch.UpdateBPT())
		h, err := batch.GetBptRootHash()
		require.NoError(t, err)
		return h
	}

	before := root()

	batch := db.Begin(true)
	for n := uint64(1); n <= 10_000; n++ {
		require.NoError(t, execute.Hold(batch, id, n, txid(n)))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	assert.Equal(t, before, root(),
		"staging is not hashed: ten thousand held messages must not move the BPT root")

	// And it really is there — the root not moving is not because nothing was
	// written.
	batch = db.Begin(false)
	defer batch.Discard()
	high, err := execute.Sighted(batch, id)
	require.NoError(t, err)
	assert.Equal(t, uint64(10_000), high)
}

// The first sighting wins. A number can be offered twice — a block re-executed,
// a healed message racing the original — and both carry the same message,
// because the number identifies it.
func TestStaging_FirstSightingWins(t *testing.T) {
	db := database.OpenInMemory(nil)
	id := stream(t)
	seedLedger(t, db, id)

	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, execute.Hold(batch, id, 5, txid(5)))
	require.NoError(t, execute.Hold(batch, id, 5, txid(99)))

	got, ok, err := execute.IDOf(batch, id, 5)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, txid(5).String(), got.String())
}

// The high-water mark does not go backwards. "This stream was behind" is what
// makes a hole below it a hole; forgetting it says the stream had never been
// behind at all.
func TestStaging_SightedDoesNotGoBackwards(t *testing.T) {
	db := database.OpenInMemory(nil)
	id := stream(t)
	seedLedger(t, db, id)

	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, execute.Hold(batch, id, 9, txid(9)))
	require.NoError(t, execute.Hold(batch, id, 3, txid(3)))

	high, err := execute.Sighted(batch, id)
	require.NoError(t, err)
	assert.Equal(t, uint64(9), high, "a later, lower sighting must not lower the mark")
}

func TestStaging_Missing(t *testing.T) {
	db := database.OpenInMemory(nil)
	id := stream(t)
	seedLedger(t, db, id)

	hold := func(ns ...uint64) *database.Batch {
		batch := db.Begin(true)
		t.Cleanup(batch.Discard)
		for _, n := range ns {
			require.NoError(t, execute.Hold(batch, id, n, txid(n)))
		}
		return batch
	}

	t.Run("nothing staged is one run", func(t *testing.T) {
		runs, err := execute.Missing(hold(), id, 10, 20, 8)
		require.NoError(t, err)
		assert.Equal(t, [][2]uint64{{11, 20}}, runs)
	})

	t.Run("holes between held numbers", func(t *testing.T) {
		runs, err := execute.Missing(hold(12, 14, 15, 19), id, 10, 20, 8)
		require.NoError(t, err)
		assert.Equal(t, [][2]uint64{{11, 11}, {13, 13}, {16, 18}, {20, 20}}, runs)
	})

	t.Run("everything held is no runs", func(t *testing.T) {
		runs, err := execute.Missing(hold(11, 12, 13, 14, 15), id, 10, 15, 8)
		require.NoError(t, err)
		assert.Empty(t, runs)
	})

	t.Run("nothing above the watermark", func(t *testing.T) {
		batch := hold(11)
		runs, err := execute.Missing(batch, id, 10, 10, 8)
		require.NoError(t, err)
		assert.Empty(t, runs, "through is not above delivered")

		runs, err = execute.Missing(batch, id, 20, 10, 8)
		require.NoError(t, err)
		assert.Empty(t, runs, "through is behind delivered")
	})

	t.Run("a delivered number is not a hole and not held", func(t *testing.T) {
		// Nothing is deleted when a number executes; the watermark is what
		// stops it counting. 11 and 12 are recorded and below Delivered.
		runs, err := execute.Missing(hold(11, 12, 15), id, 12, 15, 8)
		require.NoError(t, err)
		assert.Equal(t, [][2]uint64{{13, 14}}, runs)
	})

	t.Run("maxRuns bounds the answer", func(t *testing.T) {
		var evens []uint64
		for n := uint64(12); n <= 40; n += 2 {
			evens = append(evens, n)
		}
		batch := hold(evens...)
		runs, err := execute.Missing(batch, id, 10, 40, 3)
		require.NoError(t, err)
		assert.Equal(t, [][2]uint64{{11, 11}, {13, 13}, {15, 15}}, runs)

		runs, err = execute.Missing(batch, id, 10, 40, 0)
		require.NoError(t, err)
		assert.Empty(t, runs, "asking for none returns none")
	})
}

// Staging must be IDENTICAL on every node, because it decides what a block
// executes. It is a deterministic function of the consensus stream, so the same
// messages must produce the same state whatever order they were seen in — which
// is what lets it be agreed without being hashed.
func TestStaging_IsAFunctionOfTheMessagesAlone(t *testing.T) {
	id := stream(t)
	numbers := []uint64{9, 3, 7, 3, 12, 1, 7, 20}

	build := func(order []uint64) (uint64, [][2]uint64) {
		db := database.OpenInMemory(nil)
		seedLedger(t, db, id)
		batch := db.Begin(true)
		defer batch.Discard()
		for _, n := range order {
			require.NoError(t, execute.Hold(batch, id, n, txid(n)))
		}
		high, err := execute.Sighted(batch, id)
		require.NoError(t, err)
		runs, err := execute.Missing(batch, id, 0, high, 16)
		require.NoError(t, err)
		return high, runs
	}

	wantHigh, wantRuns := build(numbers)

	// Every rotation of the same set. A different arrival order is the only
	// thing that varies between nodes, and it must change nothing.
	for i := range numbers {
		rotated := append(append([]uint64{}, numbers[i:]...), numbers[:i]...)
		high, runs := build(rotated)
		assert.Equalf(t, wantHigh, high, "rotation %d changed the watermark", i)
		assert.Equalf(t, wantRuns, runs, "rotation %d changed the gaps", i)
	}
}
