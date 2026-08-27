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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// buildRun is the core of the staging restructure (#4169 step 4): one walk
// forward from where a stream stands, taking each number from whichever side
// holds it. Draining a staged tail behind an arrival is not a separate
// mechanism — it is the walk continuing. That is what lets the cascade go.

const noLimit = 1 << 20

// pos builds a stream position: delivered/received, holding `hold`.
func runPos(delivered, received uint64, hold ...uint64) *streamPosition {
	p := &streamPosition{delivered: delivered, received: received}
	if received > delivered {
		p.staged = make([]*url.TxID, received-delivered)
	}
	for _, n := range hold {
		p.staged[n-delivered-1] = protocol.PartitionUrl("BVN0").WithTxID([32]byte{byte(n)})
	}
	return p
}

// arr builds arrivals, all admissible.
func arr(numbers ...uint64) map[uint64]*arrival {
	m := map[uint64]*arrival{}
	for _, n := range numbers {
		m[n] = &arrival{number: n, message: &messaging.SequencedMessage{Number: n}, admissible: true}
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
	run, stage := buildRun(runPos(0, 0), arr(1, 2, 3), noLimit)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run))
	assert.Empty(t, stage)
}

func TestBuildRun_AGapStopsIt(t *testing.T) {
	// 1 and 2 are next; 4 is past a hole at 3.
	run, stage := buildRun(runPos(0, 0), arr(1, 2, 4), noLimit)
	assert.Equal(t, []uint64{1, 2}, runNumbers(run), "the run ends at the hole")
	assert.Equal(t, []uint64{4}, stagedNumbers(stage), "what is past the hole stays staged")
}

// The case that used to need the cascade: #1 arrives, and #2 and #3 are
// already held, so the stream runs to 3 in one block.
func TestBuildRun_StagedTailDrainsBehindAnArrival(t *testing.T) {
	run, stage := buildRun(runPos(0, 3, 2, 3), arr(1), noLimit)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run),
		"the staged tail drains behind the arrival — one walk, not a cascade")
	assert.Empty(t, stage)
	assert.Nil(t, run[0].staged, "#1 arrived this block, so its message is in hand")
	assert.NotNil(t, run[1].staged, "#2 was already held, so the caller loads it by ID")
}

// The mirror case: the arrival is what closes a hole ahead of held entries.
func TestBuildRun_AnArrivalFillsAGapAheadOfStagedEntries(t *testing.T) {
	// Delivered 5, holding 7 and 8, and 6 turns up now.
	run, _ := buildRun(runPos(5, 8, 7, 8), arr(6), noLimit)
	assert.Equal(t, []uint64{6, 7, 8}, runNumbers(run))
}

// The reason staging has to ask about proofs. An inadmissible message must not
// execute, so the stream must not advance over it — advancing would mark it
// delivered without running it.
func TestBuildRun_AnInadmissibleMessageStopsTheRun(t *testing.T) {
	a := arr(1, 2, 3)
	a[2].admissible = false

	run, stage := buildRun(runPos(0, 0), a, noLimit)
	assert.Equal(t, []uint64{1}, runNumbers(run), "the run stops AT the unproven message, not after it")
	assert.Equal(t, []uint64{2, 3}, stagedNumbers(stage),
		"the unproven message stays staged and is retried when its anchor lands")
}

func TestBuildRun_AppliesEachNumberOnce(t *testing.T) {
	// Already staged AND arriving again. It must appear once.
	run, stage := buildRun(runPos(0, 2, 1, 2), arr(1, 2), noLimit)
	assert.Equal(t, []uint64{1, 2}, runNumbers(run))
	assert.Empty(t, stage)

	// And a number already behind the stream is neither run nor re-staged.
	run, stage = buildRun(runPos(5, 5), arr(3, 4), noLimit)
	assert.Empty(t, run, "already delivered")
	assert.Empty(t, stage, "and not ours to record again")
}

func TestBuildRun_RespectsTheLimit(t *testing.T) {
	run, stage := buildRun(runPos(0, 0), arr(1, 2, 3, 4, 5), 3)
	assert.Equal(t, []uint64{1, 2, 3}, runNumbers(run), "one block cannot inherit an unbounded drain")
	assert.Equal(t, []uint64{4, 5}, stagedNumbers(stage), "the rest continues next block")
}

func TestBuildRun_EmptyStream(t *testing.T) {
	run, stage := buildRun(runPos(0, 0), nil, noLimit)
	assert.Empty(t, run)
	assert.Empty(t, stage)
}

// Every node must derive the same thing from the same block. The staged set is
// collected from a map, so it is sorted before it leaves.
func TestBuildRun_StagedOrderIsDeterministic(t *testing.T) {
	for i := 0; i < 20; i++ {
		_, stage := buildRun(runPos(0, 0), arr(9, 3, 7, 5), noLimit)
		require.Equal(t, []uint64{3, 5, 7, 9}, stagedNumbers(stage))
	}
}
