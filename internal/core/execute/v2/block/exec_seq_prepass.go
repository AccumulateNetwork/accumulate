// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// decideSequencedReadiness settles, before any shard runs, which sequenced
// messages in this block are next in their stream.
//
// Readiness is `partitionLedger.Delivered+1 == seq.Number` — a property of how
// far the stream has advanced, not of the message. Evaluated inside execution
// it becomes a property of SCHEDULING: two messages of one stream arriving in
// the same block, routed to different shards by their destination identities,
// would disagree about which is next depending on which shard ran first. Nodes
// configured with different shard counts would then diverge on a state hash —
// the one failure this whole design must not have.
//
// So it is decided here: once, serially, in arrival order, against the ledger
// as it stands at the start of the block, advancing a local watermark as the
// pass admits each message. That is exactly the sequence serial execution would
// have produced, and it no longer depends on when anything runs.
//
// This pass READS ONLY. It does not touch the ledger, produce messages, or
// record anything — the ledger write stays where it is for now, in
// SequencedMessage.process. Moving the write is the next step; moving the
// DECISION is what makes the write safe to move.
//
// A message the pass leaves no entry for falls back to the live check. That is
// the whole of the fallback contract, and two things land in it: cascade
// messages (#4146), generated during execution and invisible from here, and
// messages on an UNDETERMINED stream — see below.
//
// Cross-checked against the live verdict over the whole e2e suite: agreement
// on every message the pass decides, in both directions. The pass never
// wrongly admits, and no longer emits a conservative `false` either.
func (b *Block) decideSequencedReadiness(envelopes []*messaging.Envelope) {
	// Per-stream working state for this pass. Where the stream STANDS is
	// streamPosition, read once per block by positionOf (#4169 step 2); this
	// is only what the pass itself learns as it walks the envelopes.
	type streamState struct {
		// pos is where the stream stood at the start of the block.
		pos *streamPosition

		// have is the local watermark: seeded from pos and moved forward as the
		// pass admits.
		have uint64

		// held maps each of this stream's numbers the pass refused to that
		// message's hash. The executor RECORDS a refused message as pending, so
		// it joins the staged window during the block and can be drained by a
		// later delivery exactly like the ones already in it — which means its
		// verdict may have to be taken back.
		held map[uint64][32]byte

		// undetermined stops the pass from speaking for this stream. See the
		// comment at the point it is set.
		undetermined bool
	}
	streams := map[string]*streamState{}

	for _, env := range envelopes {
		messages, err := env.Normalize()
		if err != nil {
			continue // a malformed envelope is its own outcome, later
		}
		for _, msg := range messages {
			// Which stream governs this message — decided by streamOf, the
			// single statement of the rule (#4169 step 1). This pass used to
			// carry its own copy that never opened a BlockAnchor and never
			// resolved a remote stub, so it could classify as synthetic what
			// the executor classified as an anchor and gate it by the wrong
			// ledger.
			str, seq, err := b.Executor.streamOf(msg, resolveFromBatch(b.Batch))
			if err != nil || !str.ok() {
				continue // not a stream message, or its body cannot be read
			}
			key := str.ledger.String() + "|" + str.source.String()
			st, seeded := streams[key]
			if !seeded {
				pos, err := b.positionOf(str)
				if err != nil {
					// Unreadable ledger is not a verdict. Leave no entry and
					// let the executor's own load report the error.
					continue
				}
				st = &streamState{pos: pos, have: pos.delivered, held: map[uint64][32]byte{}}
				streams[key] = st
			}

			if st.undetermined {
				continue
			}

			// Key on the SEQUENCED message's hash, not the envelope
			// message's. The executor reaches isReady through a child context
			// whose message IS the sequenced message — callMessageExecutor
			// unwraps the synthetic wrapper first — so keying on the outer
			// hash stores a verdict nothing ever looks up.
			// FIRST VERDICT WINS. The same sequenced message can appear more
			// than once in a block's envelopes, and serial execution handles
			// the repeat through the already-delivered path — isReady returns
			// errors.Delivered, not "not ready". Re-deciding it here saw the
			// watermark it had itself advanced and overwrote a correct `true`
			// with a wrong `false`: measured as 6 disagreements against the
			// live check, all of them syntheticDepositTokens whose live
			// verdict was ready.
			h := seq.Hash()
			if _, seen := b.seqReady[h]; seen {
				continue
			}

			if seq.Number != st.have+1 {
				// Already delivered, or a gap. Either way not next, and the
				// watermark does not move — a later message in this block
				// cannot jump the gap. A refused message above the watermark
				// is one the executor records as pending, which puts it in
				// the window a later delivery can drain from.
				b.setSeqReady(h, false)
				if seq.Number > st.have {
					st.held[seq.Number] = h
				}
				continue
			}

			b.setSeqReady(h, true)
			st.have = seq.Number

			// A stream does not advance by one per delivery. When a message
			// delivers, SequencedMessage.process cascades: if its successor
			// is ALREADY RECEIVED — sitting undelivered in the ledger's
			// pending window — that successor is delivered too, inline, and
			// its own delivery cascades again. The stream can therefore run
			// far past what this block's envelopes contain, and where it
			// stops is decided by nextTargetsSameIdentity, which reads the
			// stored successor to compare principals.
			//
			// This pass cannot follow that without reimplementing the cascade
			// — a second copy of the rule, which is the exact failure the
			// pre-pass exists to avoid. Nor may it GUESS the drain: guessing
			// it happens is a wrong ADMIT, the one direction that corrupts a
			// watermark.
			//
			// So it stops speaking for the stream instead. Every later
			// message of this stream gets no entry and is decided live,
			// exactly as a cascade message is — and the refusals it already
			// made are taken back, for the reason given below.
			//
			// Measured: this is what the pre-pass and the live check used to
			// disagree about, and the only thing. Delivered=0 with #2 and #3
			// already received, envelopes carrying #1 and #4: the pass
			// admitted #1, the tail drained to 3 behind it, and #4 — which
			// the pass had called a gap against a watermark of 1 — was next
			// by the time it ran.
			pendingNext := st.pos.has(seq.Number + 1)
			_, heldNext := st.held[seq.Number+1]
			if pendingNext || heldNext {
				st.undetermined = true

				// Take back every refusal on this stream. A refused message
				// is recorded pending, and the drain reaches into exactly
				// that window — it re-enters the executor as a cascade
				// delivery, with the same hash, and finds a verdict decided
				// against a watermark the drain has since moved past.
				//
				// Admissions stand. A message the pass admitted delivers at
				// its arrival attempt, before any of this; a later sighting
				// of it is a repeat, which Process settles from the recorded
				// status without ever asking again.
				//
				// Measured: leaving the refusals in place was the one
				// remaining disagreement — envelopes arriving [#4, #1, #2,
				// #3] on an empty stream. #4 was refused against a watermark
				// of 0, correctly, and was still refused when #3's delivery
				// drained the tail into it.
				for _, held := range st.held {
					delete(b.seqReady, held)
				}
				clear(st.held)
			}
		}
	}
}

func (b *Block) setSeqReady(hash [32]byte, ready bool) {
	if b.seqReady == nil {
		b.seqReady = map[[32]byte]bool{}
	}
	b.seqReady[hash] = ready
}

// seqReadyFor reports a precomputed verdict, if the pre-pass reached this
// message. ok=false means "decide it live", not "not ready".
func (b *Block) seqReadyFor(hash [32]byte) (ready, ok bool) {
	if b.seqReady == nil {
		return false, false
	}
	ready, ok = b.seqReady[hash]
	return ready, ok
}
