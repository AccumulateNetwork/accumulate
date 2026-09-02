// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// Property tests over buildRun. The table tests say what it does for cases I
// thought of; these say what must hold for cases I did not.

// checkRunInvariants asserts everything that must be true of any result, for
// any input. Used by the property tests and the fuzzer alike.
func checkRunInvariants(t *testing.T, pos *streamPosition, arriving map[uint64]*arrival, limit uint64, run []runEntry, stage []*arrival) {
	t.Helper()

	// Consecutive, from where the stream stands.
	for i, e := range run {
		require.Equalf(t, pos.next()+uint64(i), e.number,
			"run[%d] must be %d, the stream's position plus %d", i, pos.next()+uint64(i), i)
	}
	assert.LessOrEqual(t, uint64(len(run)), limit, "the run is bounded")

	// CONSERVATION. Every arrival is accounted for exactly once: it ran, it
	// stayed staged, or it was already behind the stream. Nothing is invented
	// and nothing is dropped — a dropped arrival is a message that arrived and
	// then simply vanished, which no test of "what ran" would catch.
	inRun := map[uint64]bool{}
	for _, e := range run {
		inRun[e.number] = true
	}
	inStage := map[uint64]bool{}
	for _, a := range stage {
		assert.Falsef(t, inStage[a.number], "%d staged twice", a.number)
		inStage[a.number] = true
	}
	for n := range arriving {
		switch {
		case n <= pos.delivered:
			assert.Falsef(t, inRun[n] || inStage[n], "%d is already delivered and must not reappear", n)
		default:
			assert.Truef(t, inRun[n] != inStage[n],
				"arrival %d must either run or stay staged, exactly one", n)
		}
	}
	for n := range inStage {
		assert.Falsef(t, inRun[n], "%d cannot both run and stay staged", n)
	}

	// Every entry is executable: exactly one source.
	for _, e := range run {
		assert.Truef(t, (e.bundle != nil) != (e.staged != nil),
			"entry %d must have exactly one source", e.number)
	}
}

// pos and arrivals built from a seed, so the shapes vary beyond what I would
// think to write down.
func propInputs(seed uint64) (*streamPosition, map[uint64]*arrival, uint64) {
	delivered := seed % 7
	window := (seed / 7) % 9
	var hold []uint64
	for i := uint64(0); i < window; i++ {
		if (seed>>(i%13))&1 == 1 {
			hold = append(hold, delivered+1+i)
		}
	}
	pos := runPos(delivered, hold...)
	arriving := map[uint64]*arrival{}
	for i := uint64(0); i < (seed/63)%7; i++ {
		n := delivered + 1 + ((seed >> i) % 12)
		arriving[n] = &arrival{
			number:     n,
			bundle:     []messaging.Message{&messaging.SequencedMessage{Number: n}},
			admissible: (seed>>(i+3))&1 == 1,
			envIdx:     int(i),
		}
	}
	limit := 1 + (seed % 11)
	return pos, arriving, limit
}

func TestBuildRun_PropertiesHoldOverManyShapes(t *testing.T) {
	for seed := uint64(0); seed < 4000; seed++ {
		pos, arriving, limit := propInputs(seed)
		run, stage := buildRun(pos, arriving, limit)
		checkRunInvariants(t, pos, arriving, limit, run, stage)
		if t.Failed() {
			t.Fatalf("seed %d: delivered=%d received=%d held=%d arrivals=%d limit=%d",
				seed, pos.delivered, pos.received(), pos.staging.Held(pos.stream.id()), len(arriving), limit)
		}
	}
}

// Every node must derive the same run from the same state. buildRun reads a
// MAP, whose iteration order changes between runs of the same binary, so this
// is not a theoretical concern — it is the one place nondeterminism could
// enter and would not show up in a single-run test.
func TestBuildRun_IsDeterministicAcrossMapOrders(t *testing.T) {
	for seed := uint64(0); seed < 200; seed++ {
		pos, arriving, limit := propInputs(seed)
		run0, stage0 := buildRun(pos, arriving, limit)

		for again := 0; again < 8; again++ {
			// Rebuild the map so Go's iteration order differs.
			shuffled := map[uint64]*arrival{}
			for n, a := range arriving {
				shuffled[n] = a
			}
			run, stage := buildRun(pos, shuffled, limit)
			require.Equalf(t, runNumbers(run0), runNumbers(run), "seed %d: run differs between identical inputs", seed)
			require.Equalf(t, stagedNumbers(stage0), stagedNumbers(stage), "seed %d: staged set differs between identical inputs", seed)
		}
	}
}

// A run leaves the stream exactly where the next block must pick it up. If a
// run of N ends at number M, next block's run must begin at M+1 — otherwise
// blocks either repeat a message or skip one.
func TestBuildRun_NextBlockResumesWhereThisOneStopped(t *testing.T) {
	pos := runPos(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	seen := []uint64{}
	for block := 0; block < 5; block++ {
		run, _ := buildRun(pos, nil, 2)
		if len(run) == 0 {
			break
		}
		for _, e := range run {
			seen = append(seen, e.number)
		}
		last := run[len(run)-1].number
		pos = runPos(last, remaining(last, 10)...)
	}
	assert.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, seen,
		"across blocks the stream is covered exactly once, in order, with no repeat and no skip")
}

func remaining(after, to uint64) []uint64 {
	var ns []uint64
	for n := after + 1; n <= to; n++ {
		ns = append(ns, n)
	}
	return ns
}

// Degenerate and hostile inputs. These are the shapes a corrupted or crafted
// ledger could present, and buildRun must not panic or loop on any of them.
func TestBuildRun_DegenerateInputs(t *testing.T) {
	t.Run("held entries behind the watermark", func(t *testing.T) {
		// Staging holding numbers at or below Delivered is inconsistent — they
		// were delivered, so Release should have dropped them. idOf must answer
		// from the watermark and not from what the map happens to contain.
		//
		// This replaces the two array hazards the positional window invited:
		// reading backwards when received was behind delivered, and running off
		// the end when the window was shorter than received claimed. Neither is
		// expressible against a map keyed by the number itself (#4189) — but a
		// stale entry is, so that is what is pinned here.
		pos := runPos(10, 3, 8, 10, 11)
		run, stage := buildRun(pos, nil, noLimit)
		assert.Equal(t, []uint64{11}, runNumbers(run), "the run starts at 11 and ignores the stale entries")
		assert.Empty(t, stage)
		assert.False(t, pos.has(10), "at the watermark is delivered, not staged")
		assert.False(t, pos.has(3))
	})

	t.Run("a hole in the middle of what is held", func(t *testing.T) {
		// The shape the whole design turns on: 1 and 2 held, 3 missing, 4 and 5
		// held. The run must stop at the hole and not jump it.
		pos := runPos(0, 1, 2, 4, 5)
		run, _ := buildRun(pos, nil, noLimit)
		assert.Equal(t, []uint64{1, 2}, runNumbers(run), "the run stops at the first number nothing holds")
	})

	t.Run("arrival numbered zero", func(t *testing.T) {
		// Sequence numbers start at 1; 0 is invalid and must not be treated
		// as next for a stream standing at 0.
		pos := runPos(0)
		a := map[uint64]*arrival{0: {number: 0, bundle: []messaging.Message{&messaging.SequencedMessage{}}, admissible: true}}
		run, stage := buildRun(pos, a, noLimit)
		assert.Empty(t, run, "zero is not delivered+1 for any stream")
		assert.Empty(t, stage, "and it is not above the watermark either")
	})

	t.Run("watermark at the top of the range", func(t *testing.T) {
		// delivered+1 overflows. The walk must terminate rather than wrap to
		// zero and start again.
		pos := runPos(math.MaxUint64)
		run, stage := buildRun(pos, nil, noLimit)
		assert.Empty(t, run)
		assert.Empty(t, stage)
	})
}

// Go's fuzzer over the same invariants: it explores shapes the seeded
// generator does not, and it is the only test here that can find a panic I
// have not imagined.
func FuzzBuildRun(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Add(uint64(97))
	f.Add(uint64(1 << 20))
	f.Add(uint64(math.MaxUint64))
	f.Fuzz(func(t *testing.T, seed uint64) {
		pos, arriving, limit := propInputs(seed)
		run, stage := buildRun(pos, arriving, limit)
		checkRunInvariants(t, pos, arriving, limit, run, stage)
	})
}
