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
func (b *Block) executeRuns(runs []streamRun, results []*execute.ProcessResult) {
	for _, sr := range runs {
		for _, entry := range sr.run {
			if b.fatal != nil {
				return
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
			}

			if err != nil || anyPending(statuses) {
				break // this stream stops here; the rest stays for a later block
			}
		}
	}
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
