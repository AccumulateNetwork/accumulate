// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamRun is one stream's decision for this block: what runs, and what stays
// staged.
type streamRun struct {
	stream stream
	run    []runEntry
	stage  []*arrival
}

// executionOrder is the order a block's work runs in, settled before anything
// executes.
//
// Three groups, in this order, and each ordering is a decision:
//
//   - Anchors first, because an anchor extends the directory root that admits
//     synthetics. Executing them first lets a synthetic use an anchor that
//     arrived in the SAME block instead of waiting one.
//   - Synthetics next, so a deposit lands before a user transaction spends.
//     A send that would fail on a stale balance succeeds instead — strictly
//     more permissive, and deterministic either way.
//   - User envelopes last, in arrival order.
//
// Within each group, streams run in canonical source order: the directory
// first, then partitions by ID. Any fixed rule would do; what matters is that
// every node uses the same one.
type executionOrder struct {
	anchors   []streamRun
	synthetic []streamRun
	user      []int // envelope indices, in arrival order
}

// stageBlock settles the block's execution order without executing anything.
//
// It sorts the block's messages into streams, then decides each stream's run
// against where that stream stands. Sorting and deciding are separate passes
// on purpose — see the note on anchors below.
//
// NOT YET AUTHORITATIVE (#4169 step 5). Computed and discarded; the executor
// still decides everything itself.
//
// A RUN IS AN ORDER, NOT A SET OF READINESS VERDICTS, and the difference is
// load-bearing. It is tempting to read "N is in the run" as "N is ready" and
// short-circuit the live check with it. That is wrong while something else
// controls execution order. A stream's staged tail drains only when a delivery
// cascades into it, so a block whose envelopes carry #240 while the stream
// sits at #1 drains nothing — yet the run correctly contains 2..240, because
// each member becomes next as the run executes. Treat membership as readiness
// and #240's envelope executes alone, out of order. Membership becomes a
// readiness verdict only once staging drives execution order too (#4169 step
// 10), which is why those two steps cannot be separated.
//
// LIMITATION, and the reason anchors are not final here: this decides both
// groups against the anchor chain as it stands BEFORE anything executes. Once
// this is authoritative the anchor group has to be decided, executed, and only
// then the synthetic group decided — otherwise a synthetic whose proving
// anchor is in this same block is judged against a chain that does not yet
// contain it, and waits a block for nothing. Shadow mode cannot tell the
// difference because it executes nothing; step 6 has to.
func (b *Block) stageBlock(envelopes []*messaging.Envelope) (*executionOrder, error) {
	order := new(executionOrder)
	arrivals := map[string]map[uint64]*arrival{}
	streams := map[string]stream{}

	for i, env := range envelopes {
		messages, err := env.Normalize()
		if err != nil {
			continue // a malformed envelope is its own outcome, later
		}

		isUser := true
		for _, msg := range messages {
			str, seq, err := b.Executor.streamOf(msg, resolveFromBatch(b.Batch))
			if err != nil || !str.ok() {
				continue
			}
			isUser = false

			// ANCHORS ARE ORDERED HERE, NOT ADMITTED HERE. An anchor's gate
			// is a validator signature quorum, and the quorum is assembled
			// from THIS BLOCK'S OWN MESSAGES — each BlockAnchor carries one
			// signature, which the executor records as it processes it.
			// Staging runs first and sees an empty signature set for every
			// anchor: measured across the e2e suite, sigsAtStaging=0 against
			// thresholds of 2, 4 and 6, every time.
			//
			// Counting the block's signatures here would mean deduping by key
			// and checking validator membership — a second implementation of
			// signature validation, which is the failure this restructure
			// exists to remove — and over-counting would admit an
			// unauthorized anchor. So the positional run is built here and
			// each entry's quorum is decided as it executes, with the run
			// stopping at the first entry that does not deliver.
			ok := true
			if str.kind != streamAnchor {
				var err error
				ok, err = b.admissibilityOf(str, msg, seq)
				if err != nil {
					// Cannot answer the precondition, so cannot place the
					// message. Leaving it out stops the stream at it, which is
					// the conservative direction.
					continue
				}
			}

			key := str.ledger.String() + "|" + str.source.String()
			if _, seen := streams[key]; !seen {
				streams[key] = str
				arrivals[key] = map[uint64]*arrival{}
			}
			// FIRST SIGHTING WINS. The same message can appear twice in one
			// block; it applies at most once (requirement 4).
			if _, dup := arrivals[key][seq.Number]; !dup {
				arrivals[key][seq.Number] = &arrival{number: seq.Number, bundle: messages, admissible: ok, envIdx: i}
			}
		}

		if isUser {
			order.user = append(order.user, i)
		}
	}

	keys := make([]string, 0, len(streams))
	for k := range streams {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return lessStream(streams[keys[i]], streams[keys[j]])
	})

	for _, k := range keys {
		str := streams[k]
		pos, err := b.positionOf(str)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		run, stage := buildRun(pos, arrivals[k], cascadeDeliveryWindow)
		sr := streamRun{stream: str, run: run, stage: stage}
		if str.kind == streamAnchor {
			order.anchors = append(order.anchors, sr)
		} else {
			order.synthetic = append(order.synthetic, sr)
		}
	}

	return order, nil
}

// admissibilityOf answers the precondition for one arriving message, whichever
// kind of stream carries it. An inadmissible message must not execute, so the
// stream must not advance over it.
func (b *Block) admissibilityOf(str stream, outer messaging.Message, seq *messaging.SequencedMessage) (bool, error) {
	switch m := outer.(type) {
	case *messaging.BlockAnchor:
		txn, ok := seq.Message.(*messaging.TransactionMessage)
		if !ok {
			return false, errors.BadRequest.With("anchor does not carry a transaction")
		}
		return b.Executor.anchorIsAdmissible(b.Batch, m.Proof, txn.Transaction, seq.Source)

	case *messaging.SyntheticMessage:
		return b.Executor.isAdmissible(b.Batch, m.Proof)

	case *messaging.BadSyntheticMessage:
		return b.Executor.isAdmissible(b.Batch, m.Proof)

	default:
		// A bare sequenced message carries no proof of its own. For a
		// synthetic that is the replica-accepted case (#4140) and is
		// admissible; for an anchor it means no collection proof, so the
		// signature quorum decides.
		if str.kind == streamAnchor {
			txn, ok := seq.Message.(*messaging.TransactionMessage)
			if !ok {
				return false, errors.BadRequest.With("anchor does not carry a transaction")
			}
			return b.Executor.anchorIsAdmissible(b.Batch, nil, txn.Transaction, seq.Source)
		}
		return b.Executor.isAdmissible(b.Batch, nil)
	}
}

// lessStream is the canonical stream order: anchors before synthetics, the
// directory before partitions, then by partition ID.
func lessStream(a, b stream) bool {
	if a.kind != b.kind {
		return a.kind == streamAnchor
	}
	ad, bd := protocol.DnUrl().Equal(a.source), protocol.DnUrl().Equal(b.source)
	if ad != bd {
		return ad
	}
	return a.source.Compare(b.source) < 0
}
