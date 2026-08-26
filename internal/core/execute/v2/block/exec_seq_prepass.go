// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
// A message the pass never reached keeps no entry and the executor falls back to
// the live check — cascade messages (#4146) are generated during execution and
// cannot be seen from here.
//
// NOT YET AUTHORITATIVE. Cross-checked against the live verdict over the whole
// e2e suite: 891 agreements, 4 disagreements, and — the property that matters —
// ZERO in the unsafe direction. The pass never wrongly ADMITS a message; every
// disagreement is a conservative `false` where the live check says ready, which
// falls through and changes nothing.
//
// The cause of those 4 is NOT pinned. They are syntheticDepositTokens whose
// stream advanced further than the envelope set alone explains, which points at
// cascade-driven delivery the pass cannot model — but that is a hypothesis, not
// a measurement. Moving the ledger WRITE onto this verdict requires closing
// them first: a conservative `false` is harmless while the live check still
// runs, and becomes a lost delivery the moment it does not.
func (b *Block) decideSequencedReadiness(envelopes []*messaging.Envelope) {
	// Local watermark per (ledger account, source), seeded lazily from the
	// stored ledger. Keyed by the ledger URL rather than a bool, because
	// anchors and synthetics keep SEPARATE ledgers (anchor pool vs synthetic)
	// and a shared watermark would let an anchor's sequence number gate a
	// synthetic's — different streams entirely.
	type streamKey struct {
		ledger string
		source string
	}
	delivered := map[streamKey]uint64{}

	for _, env := range envelopes {
		messages, err := env.Normalize()
		if err != nil {
			continue // a malformed envelope is its own outcome, later
		}
		for _, msg := range messages {
			seq, ok := unwrapSequenced(msg)
			if !ok || seq.Source == nil {
				continue
			}

			// Which ledger governs this stream: anchors keep theirs in the
			// anchor pool, synthetics in the synthetic account.
			ledgerUrl := b.Executor.Describe.Synthetic()
			if isAnchorBody(seq.Message) {
				ledgerUrl = b.Executor.Describe.AnchorPool()
			}

			key := streamKey{ledgerUrl.String(), seq.Source.String()}

			have, seeded := delivered[key]
			if !seeded {
				var ledger protocol.SequenceLedger
				err := b.Batch.Account(ledgerUrl).Main().GetAs(&ledger)
				if err != nil {
					// Unreadable ledger is not a verdict. Leave no entry and
					// let the executor's own load report the error.
					continue
				}
				have = ledger.Partition(seq.Source).Delivered
				delivered[key] = have
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
			if seq.Number == have+1 {
				b.setSeqReady(h, true)
				delivered[key] = seq.Number
			} else {
				// Already delivered, or a gap. Either way not next, and the
				// watermark does not move — a later message in this block
				// cannot jump the gap.
				b.setSeqReady(h, false)
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

// unwrapSequenced finds the sequenced message inside an envelope's message,
// whether it travels bare or wrapped in a synthetic (the usual case:
// SyntheticMessage{ SequencedMessage{ TransactionMessage } }).
func unwrapSequenced(msg messaging.Message) (*messaging.SequencedMessage, bool) {
	switch m := msg.(type) {
	case *messaging.SequencedMessage:
		return m, true
	case *messaging.SyntheticMessage:
		if seq, ok := m.Message.(*messaging.SequencedMessage); ok {
			return seq, true
		}
	case *messaging.BadSyntheticMessage:
		if seq, ok := m.Message.(*messaging.SequencedMessage); ok {
			return seq, true
		}
	}
	return nil, false
}

// isAnchorBody reports whether a sequenced message carries an anchor, which
// decides WHICH ledger governs its stream.
func isAnchorBody(msg messaging.Message) bool {
	tm, ok := msg.(*messaging.TransactionMessage)
	return ok && tm.Transaction.Body.Type().IsAnchor()
}
