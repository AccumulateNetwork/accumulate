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
// (see stageRuns), so an anchor below its signature threshold records pending
// here, and everything behind it in that stream must wait.
//
// "DELIVERED" IS THE LEDGER MOVING, NOT A STATUS SAYING SO. This used to ask
// the returned statuses — no error, nothing pending — and that is a different
// question with a different answer. MessageIsReady turns a missing message
// into protocol.NewErrorStatus, which is neither an error return nor a pending
// status, so an entry that achieved nothing read as success; so did
// re-executing something already delivered, and so did an empty status list.
// Measured: drain rounds reported 80 deliveries each and ran to the round
// bound while the watermark did not move at all. The stream's own watermark
// cannot be fooled that way, and reading it is cheap — the block's batch
// memoizes the record, and only a read into a CHILD batch copies.
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

			var statuses []*protocol.TransactionStatus
			var bundles []*bundle
			var err error
			if entry.bundle != nil {
				statuses, bundles, err = b.processEnvelope(b.Batch, entry.bundle)
			} else {
				// Already staged, so the message is in the database rather
				// than in hand, and MessageIsReady loads and executes it —
				// the same path the old mechanism used, reached from the
				// stage's run rather than from a delivery's side effect.
				//
				// NOT the first pass. callMessageExecutor refuses an internal
				// message at pass 0, deliberately: internal types cannot be
				// marshalled, so one arriving in a submitted envelope would
				// have to be forged. The old mechanism never met that guard,
				// because queueAdditional hands its message to a LATER pass
				// of the running bundle. A run entry is internally generated
				// in exactly the same sense, so it enters at the same pass.
				//
				// Getting this wrong is silent. The guard returns an error
				// STATUS rather than an error, so the entry looked like it
				// ran: every staged entry failed, every run stopped at its
				// first one, and only freshly arrived messages were ever
				// delivered — 40 per block against 40 arriving, a backlog
				// that could not close. Found by asking the ledger whether it
				// had moved instead of asking the statuses.
				statuses, bundles, err = b.processMessages(b.Batch,
					[]messaging.Message{&internal.MessageIsReady{TxID: entry.staged}}, 1)
			}
			if err == nil {
				for _, d := range bundles {
					d.mergeIntoBlock()
				}
			}
			if entry.envIdx >= 0 {
				results[entry.envIdx] = &execute.ProcessResult{Statuses: statuses, Error: err, Shard: -1}
				ran[entry.envIdx] = true
			}

			// Did the stream actually move? Ask it. The position IS the
			// block's state of the stream (#4169 step 7), so this is the
			// same object staging decided against, advanced by whatever the
			// entry did.
			pos, perr := b.positionOf(sr.stream)
			if err != nil || perr != nil || pos.delivered < entry.number {
				break // this stream stops here; the rest stays for a later block
			}
			delivered++
		}
	}
	return delivered
}

// maxRunPerBlock bounds how far one stage may carry a stream in a single
// block, so no block inherits an unbounded run. Same value the old mechanism
// used, named for what it actually bounds: a stage's run, not a chain of
// deliveries setting each other off.
const maxRunPerBlock = 1024

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
