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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// buildRun is the core of the staging restructure (#4169 step 4): one walk
// forward from where a stream stands, taking each number from whichever side
// holds it. Draining a staged tail behind an arrival is not a separate
// mechanism — it is the walk continuing. That is what lets the cascade go.

const noLimit = 1 << 20

// runPos builds a stream position delivered to `delivered` and holding `hold`,
// backed by its own staging store.
//
// There is no `received` any more. It named a high-water mark the ledger kept
// alongside the held set, and the two could disagree; what a stream has seen is
// now the highest thing staging holds, which is one fact with one owner
// (#4189).
func runPos(t *testing.T, delivered uint64, hold ...uint64) *streamPosition {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	s := stream{kind: streamSynthetic, ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic), source: protocol.PartitionUrl("BVN0")}
	for _, n := range hold {
		require.NoError(t, execute.Hold(batch, s.id(), n, protocol.PartitionUrl("BVN0").WithTxID([32]byte{byte(n)})))
	}
	return &streamPosition{stream: s, delivered: delivered, batch: batch}
}

// arr builds arrivals, all admissible.
func arr(numbers ...uint64) map[uint64]*arrival {
	m := map[uint64]*arrival{}
	for _, n := range numbers {
		m[n] = &arrival{number: n, bundle: []messaging.Message{&messaging.SequencedMessage{Number: n}}, admissible: true}
	}
	return m
}

func runNumbers(run []runEntry) []uint64 {
	ns := make([]uint64, len(run))
	for i, e := range run {
		ns[i] = e.number
	}
	return ns
}

func stagedNumbers(stage []*arrival) []uint64 {
	ns := make([]uint64, len(stage))
	for i, a := range stage {
		ns[i] = a.number
	}
	return ns
}

func TestBuildRun_InOrderArrivals(t *testing.T) {
	run, stage := buildRun(runPos(t, 0), arr(1, 2, 3), noLimit)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run))
	assert.Empty(t, stage)
}

func TestBuildRun_AGapStopsIt(t *testing.T) {
	// 1 and 2 are next; 4 is past a hole at 3.
	run, stage := buildRun(runPos(t, 0), arr(1, 2, 4), noLimit)
	assert.Equal(t, []uint64{1, 2}, runNumbers(run), "the run ends at the hole")
	assert.Equal(t, []uint64{4}, stagedNumbers(stage), "what is past the hole stays staged")
}

// The case that used to need the cascade: #1 arrives, and #2 and #3 are
// already held, so the stream runs to 3 in one block.
func TestBuildRun_StagedTailDrainsBehindAnArrival(t *testing.T) {
	run, stage := buildRun(runPos(t, 0, 2, 3), arr(1), noLimit)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run),
		"the staged tail drains behind the arrival — one walk, not a cascade")
	assert.Empty(t, stage)
	assert.NotNil(t, run[0].bundle, "#1 arrived this block, so its envelope is in hand")
	assert.NotNil(t, run[1].staged, "#2 was already held, so the caller loads it by ID")
}

// The mirror case: the arrival is what closes a hole ahead of held entries.
func TestBuildRun_AnArrivalFillsAGapAheadOfStagedEntries(t *testing.T) {
	// Delivered 5, holding 7 and 8, and 6 turns up now.
	run, _ := buildRun(runPos(t, 5, 7, 8), arr(6), noLimit)
	assert.Equal(t, []uint64{6, 7, 8}, runNumbers(run))
}

// The reason staging has to ask about proofs. An inadmissible message must not
// execute, so the stream must not advance over it — advancing would mark it
// delivered without running it.
func TestBuildRun_AnInadmissibleMessageStopsTheRun(t *testing.T) {
	a := arr(1, 2, 3)
	a[2].admissible = false

	run, stage := buildRun(runPos(t, 0), a, noLimit)
	assert.Equal(t, []uint64{1}, runNumbers(run), "the run stops AT the unproven message, not after it")
	assert.Equal(t, []uint64{2, 3}, stagedNumbers(stage),
		"the unproven message stays staged and is retried when its anchor lands")
}

func TestBuildRun_AppliesEachNumberOnce(t *testing.T) {
	// Already staged AND arriving again. It must appear once.
	run, stage := buildRun(runPos(t, 0, 1, 2), arr(1, 2), noLimit)
	assert.Equal(t, []uint64{1, 2}, runNumbers(run))
	assert.Empty(t, stage)

	// And a number already behind the stream is neither run nor re-staged.
	run, stage = buildRun(runPos(t, 5), arr(3, 4), noLimit)
	assert.Empty(t, run, "already delivered")
	assert.Empty(t, stage, "and not ours to record again")
}

func TestBuildRun_RespectsTheLimit(t *testing.T) {
	run, stage := buildRun(runPos(t, 0), arr(1, 2, 3, 4, 5), 3)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run), "one block cannot inherit an unbounded drain")
	assert.Equal(t, []uint64{4, 5}, stagedNumbers(stage), "the rest continues next block")
}

func TestBuildRun_EmptyStream(t *testing.T) {
	run, stage := buildRun(runPos(t, 0), nil, noLimit)
	assert.Empty(t, run)
	assert.Empty(t, stage)
}

// Every node must derive the same thing from the same block. The staged set is
// collected from a map, so it is sorted before it leaves.
func TestBuildRun_StagedOrderIsDeterministic(t *testing.T) {
	for i := 0; i < 20; i++ {
		_, stage := buildRun(runPos(t, 0), arr(9, 3, 7, 5), noLimit)
		require.Equal(t, []uint64{3, 5, 7, 9}, stagedNumbers(stage))
	}
}

// A run must be exactly the consecutive numbers from where the stream stands.
// Not "sorted", not "increasing" — consecutive, with no hole, starting at
// delivered+1. Everything downstream reads it as an order to execute in, so a
// hole in the middle would apply a message out of sequence, and a wrong start
// would apply one twice or skip one entirely.
func TestBuildRun_RunIsConsecutiveFromNext(t *testing.T) {
	cases := []struct {
		name string
		pos  *streamPosition
		arr  map[uint64]*arrival
	}{
		{"all arriving", runPos(t, 0), arr(1, 2, 3, 4)},
		{"all staged", runPos(t, 0, 1, 2, 3), nil},
		{"mixed", runPos(t, 5, 7, 8), arr(6)},
		{"arrivals ahead of staged", runPos(t, 10, 11, 12), arr(13, 14)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run, _ := buildRun(c.pos, c.arr, noLimit)
			require.NotEmpty(t, run)
			assert.Equal(t, c.pos.next(), run[0].number, "a run starts where the stream stands")
			for i := 1; i < len(run); i++ {
				assert.Equalf(t, run[i-1].number+1, run[i].number,
					"entry %d must follow %d with no hole", i, i-1)
			}
		})
	}
}

// Exactly one source per entry. An entry with neither has nothing to execute;
// an entry with both is ambiguous about which the executor should use.
func TestBuildRun_EveryEntryHasExactlyOneSource(t *testing.T) {
	run, _ := buildRun(runPos(t, 0, 2, 3), arr(1), noLimit)
	require.Len(t, run, 3)
	for _, e := range run {
		hasBundle := e.bundle != nil
		hasStaged := e.staged != nil
		assert.Truef(t, hasBundle != hasStaged,
			"entry %d must have a bundle or a staged ID, never both and never neither", e.number)
		if hasStaged {
			assert.Equal(t, -1, e.envIdx, "a staged entry belongs to no envelope of this block")
		} else {
			assert.GreaterOrEqual(t, e.envIdx, 0, "an arrival remembers its envelope")
		}
	}
}

// #4169 assumption 7.4: a staged entry carries no admissibility flag because
// anything in the staged window passed its proof check when it was recorded —
// an unproven message returns Pending before ever reaching the sequence check,
// so it never enters the window. This pins the CONSEQUENCE of that claim: an
// inadmissible arrival stops a run, and a staged entry never does, whatever
// the arrivals around it say.
func TestBuildRun_StagedEntriesAreNotGatedOnAdmissibility(t *testing.T) {
	// #1 arrives and is inadmissible; the run cannot start at all.
	a := arr(1)
	a[1].admissible = false
	run, _ := buildRun(runPos(t, 0, 2, 3), a, noLimit)
	assert.Empty(t, run, "an unproven message at the head stops the stream dead")

	// #1 arrives admissible; the staged tail behind it runs without any
	// admissibility question being asked of it.
	run, _ = buildRun(runPos(t, 0, 2, 3), arr(1), noLimit)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run))
}

// The limit bounds the run, not the stream: what is cut stays available and
// the NEXT block resumes exactly where this one stopped.
func TestBuildRun_LimitCutsTheRunNotTheStream(t *testing.T) {
	pos := runPos(t, 0, 1, 2, 3, 4, 5, 6)
	run, _ := buildRun(pos, nil, 2)
	assert.Equal(t, []uint64{1, 2}, runNumbers(run))

	// Next block, the stream stands two further on; the rest is still there.
	run, _ = buildRun(runPos(t, 2, 3, 4, 5, 6), nil, 2)
	assert.Equal(t, []uint64{3, 4}, runNumbers(run), "the remainder is not lost, only deferred")
}

// A zero limit must produce nothing rather than everything — an off-by-one
// here would turn "bounded" into "unbounded" silently.
func TestBuildRun_ZeroLimitRunsNothing(t *testing.T) {
	run, stage := buildRun(runPos(t, 0), arr(1, 2, 3), 0)
	assert.Empty(t, run)
	assert.Equal(t, []uint64{1, 2, 3}, stagedNumbers(stage), "and nothing is lost")
}

// WHY staging has to be durable, stated as a test.
//
// A block delivers the contiguous run from Delivered+1 taken from this block's
// arrivals AND from what is already held. So two nodes holding different things
// execute different runs from the same block — different Delivered, different
// account state, different BPT root. That is a divergent block hash, not a node
// briefly behind, and healing cannot repair it because healing is asynchronous
// and the divergence is immediate.
//
// This is what "empty on restart, refilled by healing" would have cost (#4188).
func TestBuildRun_WhatIsHeldDecidesWhatExecutes(t *testing.T) {
	arriving := arr(2) // the message that closes the gap

	// A node that kept its staging runs the whole tail behind the arrival.
	kept, _ := buildRun(runPos(t, 1, 3, 4, 5, 6), arriving, noLimit)
	require.Equal(t, []uint64{2, 3, 4, 5, 6}, runNumbers(kept))

	// A node that lost it delivers the arrival alone, from the same block.
	lost, _ := buildRun(runPos(t, 1), arriving, noLimit)
	require.Equal(t, []uint64{2}, runNumbers(lost),
		"same block, same arrival, different run — which is a different block hash")

	require.NotEqual(t, len(kept), len(lost),
		"if these ever match, staging no longer decides what executes and it could be rebuilt instead of restored")
}
