// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// executeRuns runs each stream's run, in the order staging settled, and
// reports each entry's outcome against the envelope it arrived in.
//
// A run STOPS at the first entry that does not deliver. That is not tidiness:
// a run is a contiguous sequence, so if entry N does not deliver then N+1 is
// not next, and executing it would apply a message out of order. The case that
// forces it is anchors — staging orders them but cannot judge their quorum
// (see stageBlock), so an anchor below its signature threshold records pending
// here, and everything behind it in that stream must wait.
//
// Streams are independent: one stopping does not stop another.
// executeRuns returns how many entries DELIVERED, so a caller can tell whether
// another round is worth running.
func (b *Block) executeRuns(runs []streamRun, results []*execute.ProcessResult, ran map[int]bool) int {
	delivered := 0
	for _, sr := range runs {
		for _, entry := range sr.run {
			if b.fatal != nil {
				return delivered
			}

			bundle := entry.bundle
			if bundle == nil {
				// Already staged, so the message is in the database rather
				// than in hand. MessageIsReady loads and executes it — the
				// same path the cascade used, reached from the run instead of
				// from a delivery's side effect.
				bundle = []messaging.Message{&internal.MessageIsReady{TxID: entry.staged}}
			}

			statuses, bundles, err := b.processEnvelope(b.Batch, bundle)
			if err == nil {
				for _, d := range bundles {
					d.mergeIntoBlock()
				}
			}
			if entry.envIdx >= 0 {
				results[entry.envIdx] = &execute.ProcessResult{Statuses: statuses, Error: err, Shard: -1}
				ran[entry.envIdx] = true
			}

			if err != nil || anyPending(statuses) {
				break // this stream stops here; the rest stays for a later block
			}
			delivered++
		}
	}
	return delivered
}

// anyPending reports whether a message came back pending, which for a run
// entry means the stream did not advance over it.
func anyPending(statuses []*protocol.TransactionStatus) bool {
	for _, st := range statuses {
		if st != nil && st.Pending() {
			return true
		}
	}
	return false
}

// maxDrainRounds bounds how many times a block re-stages what its own
// execution revealed. Two is normally enough — envelopes are processed once,
// so there is one wave of newly recorded messages — but a delivery can record
// further messages of its own, so the loop runs until a round delivers nothing
// and stops either way. A bound rather than a bare loop because a round that
// somehow always reports progress must not hang a block.
const maxDrainRounds = 8

// drainRevealed keeps draining while the block's own execution keeps making
// more of a stream drainable. See ProcessAll.
func (b *Block) drainRevealed(drain func() (int, error)) {
	for i := 0; i < maxDrainRounds; i++ {
		n, err := drain()
		if err != nil || n == 0 {
			return
		}
	}
}
