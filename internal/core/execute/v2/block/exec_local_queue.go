// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// #4146: a synthetic whose destination routes back to the producing
// partition used to go the full distance — a sequence number, a proof
// continued to a DN-anchored root, dispatch to itself, and anchor-pool
// validation on arrival — for an entirely local operation. Now it goes on a
// persisted queue and executes at the start of the next block as an ordinary
// message with its own principal. No sequence number, no position on the
// synthetic main chain (so it consumes no collection-proof span), no
// dispatch, and no anchoring dependency.
//
// Next block, not the same block: a synthetic that produces synthetics
// simply creates ordered work in the following block — no cascade, no
// termination bound. That falls out of the geometry: the queues drain at the
// START of a block and are written at the END of one, so nothing queued
// while draining can execute before the next block.
//
// Two queues, one rule each:
//   - LocalDeliveryQueue holds locally produced messages, written by the
//     block-end split below. Each entry is destination.WithTxID(msgHash), so
//     the drain can re-check routing without re-deriving the destination
//     from the message type.
//   - (removed) CascadeDeliveryQueue held ready sequenced messages whose
//     execution
//     the MessageIsReady cascade deferred because they belong to a different
//     identity (#4145 hazard iii). Entries are the pending txids the ledger
//     already tracks.

// routesToSelf reports whether the destination routes to this partition.
func (x *Executor) routesToSelf(dest *url.URL) bool {
	part, err := x.Router.RouteAccount(dest)
	return err == nil && strings.EqualFold(part, x.Describe.PartitionId)
}

// splitLocalDeliveries separates the block's produced messages into locals —
// stored and queued for the next block — and remotes, which the caller
// sequences and dispatches. The input must already be in canonical order
// (#4144); the queue preserves it, which makes the drain deterministic.
func (b *Block) splitLocalDeliveries(produced []*ProducedMessage) ([]*ProducedMessage, error) {
	var remote []*ProducedMessage
	var queued []*url.TxID
	for _, p := range produced {
		if !b.Executor.routesToSelf(p.Destination) {
			remote = append(remote, p)
			continue
		}

		// The remote path pads exactly-64-byte bodies and headers in
		// buildSynthTxn (adjust64). A local delivery skips buildSynthTxn, so
		// pad HERE, before the hash is used for the store and the queue —
		// otherwise the drain rejects the message (BodyIs64Bytes) with no
		// refund, after the source-side debit already stood (#4154).
		err := adjust64(p)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("adjust local delivery: %w", err)
		}

		// Store the message so the drain can load it. It takes no sequence
		// number and no position on the synthetic main chain — the audit
		// trail is the producer's Produced() index (written by didProduce)
		// and the destination's main chain when it executes.
		err = b.Batch.Message(p.Message.Hash()).Main().Put(p.Message)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store local delivery: %w", err)
		}
		queued = append(queued, p.Destination.WithTxID(p.Message.Hash()))
	}

	if len(queued) > 0 {
		err := b.Batch.Account(b.Executor.Describe.Synthetic()).LocalDeliveryQueue().Add(queued...)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("queue local deliveries: %w", err)
		}
		// The queue write must survive even if nothing else happened this
		// block — an "empty" discard would re-produce the same messages
		// forever (#4155).
		b.State.LocalDeliveries += len(queued)
	}
	return remote, nil
}

// drainDeliveryQueues executes the local deliveries queued for this block,
// before any of the block's own envelopes. The queue is cleared before
// execution, so anything a drained message produces lands in the NEXT block's
// queue.
//
// There is no cascade half any more (#4169). Continuing an inbound stream is
// not a queue of work one delivery hands to the next — it is a stage, decided
// in full before anything runs. Local deliveries stay because they are a
// different thing: messages we produced for ourselves, carrying no position in
// any stream, which no stage has a place for.
func (b *Block) drainDeliveryQueues() error {
	synth := b.Batch.Account(b.Executor.Describe.Synthetic())

	locals, err := synth.LocalDeliveryQueue().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load local delivery queue: %w", err)
	}
	if len(locals) == 0 {
		return nil
	}

	err = synth.LocalDeliveryQueue().Put(nil)
	if err != nil {
		return errors.UnknownError.WithFormat("clear local delivery queue: %w", err)
	}

	var msgs []messaging.Message
	for _, id := range locals {
		msg, err := b.Batch.Message(id.Hash()).Main().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load queued local delivery %v: %w", id, err)
		}

		// Routing can change between produce and drain — globals change on
		// block boundaries. A message queued as local whose destination is no
		// longer local is promoted to the cross-partition path: it joins this
		// block's produced set and is sequenced at block end. Its producer
		// attribution is not carried by the queue, so it takes the
		// producer-less slot in the canonical order — deterministic, keyed by
		// message hash.
		if !b.Executor.routesToSelf(id.Account()) {
			b.produced = append(b.produced, &ProducedMessage{Destination: id.Account(), Message: msg})
			continue
		}

		msgs = append(msgs, &internal.PseudoSynthetic{Message: msg})
	}

	if len(msgs) == 0 {
		return nil
	}

	// The queue CLEAR must survive even if every drained message fails
	// statelessly — an "empty" discard would drain the same entries every
	// block (#4155).
	b.State.LocalDeliveries += len(msgs)

	// Pass 1: queued messages are effects of already-committed work, and
	// internal message types are only admitted past pass zero.
	statuses, bundles, err := b.processMessages(b.Batch, msgs, 1)
	if err != nil {
		return errors.UnknownError.WithFormat("drain delivery queues: %w", err)
	}
	for _, d := range bundles {
		d.mergeIntoBlock()
	}

	// A drained message was never in any mempool and has no submitter to
	// notice a failure — a silent error status here is a deposit that
	// vanished without a trace (#4154). The status is its only witness.
	for _, st := range statuses {
		if st.Error != nil {
			b.Executor.logger.Error("Local delivery failed",
				"id", st.TxID, "error", st.Error, "block", b.Index)
		}
	}
	return nil
}
