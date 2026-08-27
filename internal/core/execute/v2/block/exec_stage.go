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
// classified is a block's messages sorted into streams, with nothing decided.
// Sorting happens once; deciding happens once per group per round, because
// both the anchor chain and the streams themselves move during the block.
type classified struct {
	streams  map[string]stream
	arrivals map[string]map[uint64]*arrival
	user     []int
}

func (b *Block) classify(envelopes []*messaging.Envelope) *classified {
	c := &classified{streams: map[string]stream{}, arrivals: map[string]map[uint64]*arrival{}}
	arrivals, streams := c.arrivals, c.streams

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

			key := str.ledger.String() + "|" + str.source.String()
			if _, seen := streams[key]; !seen {
				streams[key] = str
				arrivals[key] = map[uint64]*arrival{}
			}
			// FIRST SIGHTING WINS. The same message can appear twice in one
			// block; it applies at most once (requirement 4).
			if _, dup := arrivals[key][seq.Number]; !dup {
				arrivals[key][seq.Number] = &arrival{number: seq.Number, bundle: messages, envIdx: i, classifier: msg, seq: seq}
			}
		}

		if isUser {
			c.user = append(c.user, i)
		}
	}
	return c
}

// stageRuns decides one kind of stream's runs AT THE MOMENT IT IS CALLED, and
// is meant to be called more than once per block.
//
// Twice over, the answer depends on when it is asked:
//
//   - A synthetic's admissibility is read from the directory anchor chain, and
//     the anchor group EXTENDS that chain. Deciding synthetics before anchors
//     run judges them against a chain missing this block's anchors.
//   - A stream's position moves as the block executes, and a message recorded
//     pending by an envelope processed this block becomes drainable within
//     this block. A run fixed before any of that cannot see it.
//
// The second one is not theoretical. Measured on TestNoLaggingChannels with
// runs decided once per block: delivery settled into exact lockstep with
// arrival — 40 in, 40 out, every block — leaving one block's arrivals of lag
// that never closed. Baseline drains those because the cascade re-reads the
// ledger after every delivery. This is the same re-read, once per round rather
// than once per message, which is where the cascade's O(n^2) came from.
func (b *Block) stageRuns(c *classified, kind streamKind) ([]streamRun, error) {
	keys := make([]string, 0, len(c.streams))
	for k, str := range c.streams {
		if str.kind == kind {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return lessStream(c.streams[keys[i]], c.streams[keys[j]])
	})

	var runs []streamRun
	for _, k := range keys {
		str := c.streams[k]
		pos, err := b.positionOf(str)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		arriving := map[uint64]*arrival{}
		for n, a := range c.arrivals[k] {
			ok := true
			if kind != streamAnchor {
				// ANCHORS ARE ORDERED HERE, NOT ADMITTED HERE. An anchor's
				// gate is a validator signature quorum assembled from THIS
				// BLOCK'S OWN MESSAGES — each BlockAnchor carries one
				// signature, recorded as the executor processes it. Staging
				// sees an empty signature set for every anchor: measured
				// across the e2e suite, sigsAtStaging=0 against thresholds of
				// 2, 4 and 6, every time. Counting the block's signatures here
				// would mean deduping by key and checking validator
				// membership — a second implementation of signature
				// validation — and over-counting would admit an unauthorized
				// anchor. So the run is positional and each entry's quorum is
				// decided as it executes.
				ok, err = b.admissibilityOf(str, a.classifier, a.seq)
				if err != nil {
					continue
				}
			}
			a.admissible = ok
			arriving[n] = a
		}

		run, stage := buildRun(pos, arriving, cascadeDeliveryWindow)
		runs = append(runs, streamRun{stream: str, run: run, stage: stage})
	}
	return runs, nil
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
