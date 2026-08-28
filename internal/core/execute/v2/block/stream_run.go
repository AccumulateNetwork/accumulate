// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// arrival is one of this stream's messages that turned up in this block.
type arrival struct {
	number uint64

	// bundle is the WHOLE envelope's normalized messages, not just the one
	// that classified as a stream message. An envelope's messages are
	// processed together — a transaction and the signatures for it — and
	// executing one alone fails checkForUnsignedTransactions with "message
	// bundle is missing a transaction".
	bundle []messaging.Message

	// envIdx is the envelope this arrived in, so its outcome can be reported
	// against the right entry of ProcessAll's results.
	envIdx int

	// classifier and seq let admissibility be answered later, when this
	// stream's group takes its turn, rather than while sorting.
	classifier messaging.Message
	seq        *messaging.SequencedMessage

	// admissible is whether the message is proven to have come from its
	// source (see Executor.isAdmissible). Only ARRIVING messages carry this:
	// anything already staged passed the proof check when it was recorded,
	// because an unproven message returns Pending before ever reaching the
	// sequence check, so it never enters the staged window. And a chain is
	// append-only, so admissible never becomes inadmissible.
	admissible bool
}

// runEntry is one message of a run, in the order it must execute. Exactly one
// of message and staged is set: message for something that arrived this block,
// staged for something already held, which the caller loads by ID.
type runEntry struct {
	number uint64
	bundle []messaging.Message
	staged *url.TxID

	// envIdx is the envelope this arrived in, or -1 for a staged entry, which
	// belongs to no envelope of this block.
	envIdx int
}

// buildRun returns the contiguous run of a stream that this block can execute,
// and which arrivals stay staged.
//
// The run starts where the stream stands and walks forward, taking each number
// from whichever side holds it — this block's arrivals or the staged window —
// and stops at the first number that is missing, inadmissible, or past the
// limit. Draining the staged tail behind an arrival is not a second mechanism
// firing — it is this walk continuing. That is the whole reason a stage can be
// decided in advance: its extent is a property of the stream's state, not of
// what each delivery happens to set off.
//
// It stops for THREE reasons, and conflating them is how this goes wrong:
//
//   - Missing. Nothing holds this number. The stream waits.
//   - Inadmissible. The message is here but not yet proven. It must not
//     execute, so the stream must not advance over it — advancing would mark
//     it delivered without running it, which is a lost delivery. This is why
//     staging has to ask about proofs at all (#4169 step 3).
//   - Limit. The run is bounded so one block cannot inherit an unbounded
//     drain. What is left stays staged and continues next block.
//
// Pure: it reads the position, it does not advance it. The caller advances the
// stream over the run it returns.
func buildRun(pos *streamPosition, arriving map[uint64]*arrival, limit uint64) (run []runEntry, stage []*arrival) {
	taken := map[uint64]bool{}

	for n := pos.next(); uint64(len(run)) < limit; n++ {
		switch a, ok := arriving[n]; {
		case ok && !a.admissible:
			// Here, but not yet proven. The run ends; it stays staged and is
			// retried when its anchor lands.
			goto done

		case ok:
			run = append(run, runEntry{number: n, bundle: a.bundle, envIdx: a.envIdx})
			taken[n] = true

		default:
			// Not in this block — is it already staged?
			id, held := pos.idOf(n)
			if !held {
				goto done
			}
			run = append(run, runEntry{number: n, staged: id, envIdx: -1})
		}
	}

done:
	// Everything that arrived and was not taken stays staged, unless it is
	// already behind the stream, in which case it has been delivered and is
	// not ours to record again.
	for n, a := range arriving {
		if taken[n] || n <= pos.delivered {
			continue
		}
		stage = append(stage, a)
	}

	// Sorted because the source is a map. Recording staged entries places each
	// by its number, so order does not change the resulting state today — but
	// every node must derive the same thing from the same block (requirement
	// 8), and leaving a map's iteration order in the output is a standing
	// invitation for that to stop being true.
	sort.Slice(stage, func(i, j int) bool { return stage[i].number < stage[j].number })
	return run, stage
}
