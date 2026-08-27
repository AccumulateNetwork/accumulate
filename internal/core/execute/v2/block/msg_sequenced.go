// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[SequencedMessage](&messageExecutors, messaging.MessageTypeSequenced)
}

// SequencedMessage records the sequence metadata and executes the message
// inside.
type SequencedMessage struct{ TransactionMessage }

func (x SequencedMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// Check the wrapper
	seq, err := x.check(batch, ctx)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate the inner message
	_, err = ctx.callMessageValidator(batch, seq.Message)
	return nil, errors.UnknownError.Wrap(err)
}

func (x SequencedMessage) check(batch *database.Batch, ctx *MessageContext) (*messaging.SequencedMessage, error) {
	seq, ok := ctx.message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSequenced, ctx.message.Type())
	}

	// Basic validation
	if seq.Message == nil {
		return nil, errors.BadRequest.With("missing message")
	}

	var missing []string
	if seq.Source == nil {
		missing = append(missing, "source")
	}
	if seq.Destination == nil {
		missing = append(missing, "destination")
	}
	if seq.Number == 0 {
		missing = append(missing, "sequence number")
	}
	if len(missing) > 0 {
		return nil, errors.BadRequest.WithFormat("invalid synthetic transaction: missing %s", strings.Join(missing, ", "))
	}

	if !ctx.Executor.Describe.NodeUrl().Equal(seq.Destination) {
		// Log loudly: a sequenced message landing on the wrong partition means
		// something upstream submitted into the wrong DAG (#4111 diagnostics —
		// dn-destined anchors were observed committing in a BVN's blocks). The
		// rejection itself was previously invisible (inner status error).
		ctx.Executor.logger.Info("Wrong-partition sequenced message rejected",
			"expected", ctx.Executor.Describe.NodeUrl(),
			"got", seq.Destination,
			"source", seq.Source,
			"seq", seq.Number)
		return nil, errors.BadRequest.WithFormat("invalid destination: expected %v, got %v", ctx.Executor.Describe.NodeUrl(), seq.Destination)
	}

	// Sequenced messages must either be synthetic or anchors
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		isAnchor, err := x.isAnchor(batch, ctx, seq)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if !isAnchor {
			return nil, errors.BadRequest.WithFormat("invalid payload for sequenced message")
		}
	}

	// Load the transaction
	if !ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		if txn, ok := seq.Message.(*messaging.TransactionMessage); ok {
			_, err := x.resolveTransaction(batch, txn)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
		}
	}

	return seq, nil
}

func (x SequencedMessage) isAnchor(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
	msg, ok := seq.Message.(*messaging.TransactionMessage)
	switch {
	case ok && msg.Transaction.Body.Type().IsAnchor():
		return true, nil

	case !ok,
		!ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled(),
		msg.Transaction.Body.Type() != protocol.TransactionTypeRemote:
		return false, nil

	}

	txn, err := ctx.getTransaction(batch, msg.Hash())
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	return txn.Body.Type().IsAnchor(), nil
}

func (x SequencedMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// TODO Update the block state?

	// Process the message
	seq, err := x.check(batch, ctx)
	var delivered bool
	if err == nil {
		delivered, err = x.process(batch, ctx, seq)
	}

	s := errors.Delivered
	if !delivered {
		s = errors.Pending
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, s, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

// ReadyReturnedPending counts violations of the invariant asserted in
// SequencedMessage.process: a READY sequenced message must never come back
// pending. It is a counter rather than a panic because a false positive must
// not take down a validator, and a counter rather than only a log line because
// the sharded-delivery design rests on this being zero — a test has to be able
// to assert it, not grep for it.
var ReadyReturnedPending atomic.Int64

// SequencedReadyExecuted counts sequenced messages that passed the readiness
// gate and therefore executed. It exists so a test asserting
// ReadyReturnedPending == 0 can also prove it EXERCISED the path: a workload
// that drains between sends produces no pending sequenced messages at all, and
// the assertion passes without having tested anything. Caught by inverting the
// invariant and watching the test still pass.
var SequencedReadyExecuted atomic.Int64

// SequencedSyntheticExecuted is the non-anchor subset of the above. Anchors are
// sequenced too and vastly outnumber synthetics on an idle network, so a
// coverage assertion on the total can be satisfied entirely by anchor traffic
// while no synthetic is ever delivered. Classified from the transaction body
// type, which is free — deliberately NOT by calling isAnchor, which would add a
// ledger load to the hot path for the benefit of a test.
var SequencedSyntheticExecuted atomic.Int64

func (x SequencedMessage) process(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
	// Check if the message is ready to process
	ready, err := x.isReady(batch, ctx, seq)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	var st *protocol.TransactionStatus
	if ready {
		// Copy to avoid issues with resolving remote transactions. If the
		// transaction is a placeholder (a remote transaction), the executor
		// will resolve the full transaction and replace the placeholder. If we
		// don't copy, that causes the sequenced message to change, which
		// changes its hash, which causes problems with recording it in the
		// database.
		msg := seq.Message
		if ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
			msg = msg.CopyAsInterface().(messaging.Message)
		}

		// Process the message within
		st, err = ctx.callMessageExecutor(batch, msg)
	} else {
		// Mark the message as pending
		ctx.Executor.logger.Debug("Pending sequenced message", "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4), "module", "synthetic")
		st, err = ctx.childWith(seq.Message).recordPending(batch)
	}
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	if st == nil {
		err = batch.Commit()
		return false, errors.UnknownError.Wrap(err)
	}

	// INVARIANT (#4145 sharded delivery): a message that was READY and therefore
	// executed never comes back pending — `st.Pending()` is true only for the
	// !ready branch, which runs recordPending and executes nothing.
	//
	// This matters beyond tidiness. Sharding synthetic delivery requires the
	// ledger owner to decide pending-vs-delivered BEFORE dispatching the
	// transaction to its destination shard, which is only possible if the
	// answer is `!ready` — derivable from the ledger alone — rather than a
	// property of the execution result. Measured across the e2e suite: 376
	// pending messages, every one of them ready=false, none ready=true.
	//
	// It holds because the proof anchor is verified in SyntheticMessage.process
	// BEFORE this executor runs; an unanchored message returns errors.Pending
	// there and never reaches here. So by this point a message is either
	// in-sequence and executable, or out of sequence.
	//
	// Absence over one suite is evidence, not proof. Say so loudly if it ever
	// breaks: a violation means the watermark advanced on a transaction that
	// did not actually deliver, and the sharded design would be unsound.
	if ready {
		SequencedReadyExecuted.Add(1)
		if tm, ok := seq.Message.(*messaging.TransactionMessage); ok &&
			!tm.Transaction.Body.Type().IsAnchor() {
			SequencedSyntheticExecuted.Add(1)
		}
	}
	if ready && st.Pending() {
		ReadyReturnedPending.Add(1)
		ctx.Executor.logger.Error(
			"INVARIANT VIOLATED: a ready sequenced message returned pending",
			"source", seq.Source, "seq", seq.Number,
			"hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
			"module", "synthetic")
	}

	// Update the ledger
	ledger, err := x.updateLedger(batch, ctx, seq, st.Pending())
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	if !st.Delivered() {
		return false, nil
	}

	// Queue the pending tail behind this message. Same identity: inline, into
	// the running bundle, as always — the inline delivery's own process()
	// then walks the rest of the tail recursively. A DIFFERENT identity
	// defers to the next block's cascade queue (#4146): the cascade is a
	// stream property, not an identity one, and under sharded execution
	// (#4145 hazard iii) it must not widen a bundle's identity set
	// mid-execution. Anchor streams always take the inline path — every
	// anchor's principal is the local anchor pool.
	//
	// The cascade schedules the whole CONTIGUOUS received run (bounded), not
	// just the immediate successor. Each queued message becomes its own
	// bundle next block — exactly as safe as the same messages arriving
	// fresh in that block — so a backlog drains cascadeDeliveryWindow per
	// block instead of one per block. One per block was a real ceiling: in
	// run 20260824T041626Z at 10 tps, a ~600-message BVN2→BVN1 backlog
	// behind a chaos pause drained at 1.04 per block, barely above its own
	// refill rate, with delivery capped at the block rate (#4163).
	next, ok := ledger.Get(seq.Number + 1)
	if ok && x.nextTargetsSameIdentity(batch, next, seq) {
		ctx.queueAdditional(&internal.MessageIsReady{TxID: next})
	} else if ok {
		queue := batch.Account(ctx.Executor.Describe.Synthetic()).CascadeDeliveryQueue()
		err = scheduleCascadeRun(queue, ledger, seq.Number)
		if err != nil {
			return false, errors.UnknownError.WithFormat("queue cascade delivery: %w", err)
		}
	}

	return true, nil
}

// MaxPendingSequenced bounds how far past the delivery point a received
// sequenced message may be RECORDED in the pending window. The window lives
// inline in the ledger record, so its length is the marshal cost of every
// subsequent update; 4096 keeps the worst-case record ~300KB and the drain
// linear, while leaving four cascade windows of runway. Receipts beyond it
// are refused (deterministically) and heal later as a produced>received
// tail.
const MaxPendingSequenced = 4 * cascadeDeliveryWindow

// cascadeDeliveryWindow bounds how many contiguous pending successors one
// delivery schedules into the next block's cascade queue. It caps the extra
// bundles a block can inherit per stream while still letting a backlog drain
// at window-rate rather than one per block.
//
// The window is a PER-BLOCK quantum, so the drain ceiling in messages/second
// is window ÷ block interval — it shrank 8x when blocks went from one per
// certificate to one per committed leader group (#4164). At 32 per block and
// 3s blocks the ceiling was ~10/s, and one overloaded stream (a stale-read
// retry storm feeding ~27/s, #4163 defect 7) buried BVN2→BVN1 under a
// 33,000-message backlog that could never drain. 1024 per block ≈ 340/s at
// the 3s interval — above any sustainable per-stream arrival rate, while
// still bounding the bundles a single block can inherit.
// TestNoLaggingChannels pins the property: a backlogged channel drains at
// backlog scale per block, not a small fixed quantum.
const cascadeDeliveryWindow = 1024

// scheduleCascadeRun adds the contiguous received run after `after` to the
// cascade queue, up to cascadeDeliveryWindow entries, stopping at the first
// entry already queued (an earlier delivery this block scheduled the rest).
func scheduleCascadeRun(queue values.List[*url.TxID], ledger *protocol.PartitionSyntheticLedger, after uint64) error {
	queued, err := queue.Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load cascade queue: %w", err)
	}
	inQueue := func(id *url.TxID) bool {
		for _, q := range queued {
			if q.Equal(id) {
				return true
			}
		}
		return false
	}
	for n := after + 1; n <= after+cascadeDeliveryWindow; n++ {
		id, ok := ledger.Get(n)
		if !ok {
			return nil // the contiguous received run ends here
		}
		if inQueue(id) {
			return nil // an earlier delivery this block already scheduled the rest
		}
		err = queue.Add(id)
		if err != nil {
			return errors.UnknownError.WithFormat("queue cascade delivery: %w", err)
		}
	}
	return nil
}

// nextTargetsSameIdentity reports whether the NEXT pending message's inner
// principal shares the current message's identity — the test that decides
// inline delivery vs the next-block cascade queue.
//
// next comes from the pending ledger, whose IDs are Destination.WithTxID —
// the LOCAL PARTITION url, not the principal. Comparing that account against
// the principal compared partition to principal and never matched for user
// synthetics (#4153): the inline branch was dead, and a pending tail drained
// at ONE message per stream per block, which cannot converge under inflow.
// The identity equality it does establish is the anchor case — every
// anchor's principal is the local anchor pool, under the partition identity
// — which stays the fast path. For everything else the real principal lives
// in the stored message.
func (x SequencedMessage) nextTargetsSameIdentity(batch *database.Batch, next *url.TxID, seq *messaging.SequencedMessage) bool {
	if next.Account().RootIdentity().Equal(seq.Message.ID().Account().RootIdentity()) {
		return true // the anchor fast path
	}

	msg, err := batch.Message(next.Hash()).Main().Get()
	if err != nil {
		return false // unknown → the cascade queue, the conservative lane
	}
	nextSeq, ok := msg.(*messaging.SequencedMessage)
	if !ok {
		return false
	}
	return nextSeq.Message.ID().Account().RootIdentity().Equal(seq.Message.ID().Account().RootIdentity())
}

func (x SequencedMessage) isReady(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
	// Prefer the verdict the block settled before execution began (#4145).
	// Deciding readiness here would make it depend on when this shard ran; the
	// pre-pass decided it once, serially, in arrival order.
	//
	// A message with no entry falls through to the live check below, exactly
	// as before. Two things land there: a cascade message (#4146), generated
	// during execution and invisible to a pass that runs before it, and a
	// message on a stream the pre-pass stopped speaking for because a drain
	// it cannot model may move the stream past it.
	//
	// The "already delivered" case still has to be answered from the ledger,
	// because its caller distinguishes it by ERROR, not by the bool.
	if ready, ok := ctx.Block.seqReadyFor(ctx.message.Hash()); ok && ready {
		return true, nil
	}

	// Load the ledger
	isAnchor, ledger, err := x.loadLedger(batch, ctx, seq)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	partitionLedger := ledger.Partition(seq.Source)

	// If the sequence number is old, mark it already delivered
	typ := "synthetic message"
	if isAnchor {
		typ = "anchor"
	}
	if seq.Number <= partitionLedger.Delivered {
		return false, errors.Delivered.WithFormat("%s has been delivered", typ)
	}

	// If the transaction is out of sequence, mark it pending
	if partitionLedger.Delivered+1 != seq.Number {
		ctx.Executor.logger.Debug("Out of sequence message",
			"hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
			"seq-got", seq.Number,
			"seq-want", partitionLedger.Delivered+1,
			"source", seq.Source,
			"destination", seq.Destination,
			"type", typ,
			"hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
		)
		return false, nil
	}

	return true, nil
}

func (x SequencedMessage) updateLedger(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage, pending bool) (*protocol.PartitionSyntheticLedger, error) {
	// Load the ledger
	isAnchor, ledger, err := x.loadLedger(batch, ctx, seq)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	partLedger := ledger.Partition(seq.Source)

	// This should never happen, but if it does Add will panic
	if pending && seq.Number <= partLedger.Delivered {
		msg := "synthetic messages"
		if isAnchor {
			msg = "anchors"
		}
		return nil, errors.FatalError.WithFormat("%s processed out of order: delivered %d, processed %d", msg, partLedger.Delivered, seq.Number)
	}

	// Bound the pending window. Per-message cost is O(total backlog), so a big
	// backlog drains in O(backlog^2) — run 20260824T051249Z's 33,000-message
	// backlog collapsed the drain to ~4/s, below even the cascade window's
	// allowance: the serial search Paul called.
	//
	// The cost is the READ, not the write. This function reads the ledger with
	// GetAs inside the message's own child batch, and a child does not share
	// its parent's value — the read deep copies the whole SyntheticLedger,
	// every stream and every pending entry. Writes are pointer assignments
	// into the parent record and marshal once at the block's commit.
	// TestSequenceLedgerCostIsPerRead pins it: across backlogs of 100 to
	// 16,000, `put` and `commit` stay flat at ~0.2us and ~1.5us while the read
	// runs 1.1us to 78.6us.
	//
	// Which decides the fix, and rules out the obvious one. Splitting the
	// ledger into one record per stream does NOT help: a stream's own backlog
	// is still copied on every one of its messages. Reading the ledger ONCE
	// PER BLOCK does, and needs no layout change — measured at a 16,000
	// backlog, 80.8us per message becomes 0.32us. That is what #4169's decide
	// pass does, and it supersedes #4164's keyed-sub-record plan.
	//
	// Until then, refusing to RECORD a receipt far beyond the delivery point
	// is deterministic (same rule, same state on every validator) and converts
	// unbounded receipt-state growth into a produced>received tail at the
	// source, which the reconcile machinery already heals once delivery
	// catches up.
	if pending && seq.Number > partLedger.Delivered+MaxPendingSequenced {
		ctx.Executor.logger.Debug("Refusing to record far-future sequenced message",
			"seq", seq.Number, "delivered", partLedger.Delivered,
			"window", MaxPendingSequenced, "source", seq.Source)
		return partLedger, nil
	}

	// The ledger's Delivered number needs to be updated if the transaction
	// succeeds or fails
	if partLedger.Add(!pending, seq.Number, seq.ID()) {
		err = batch.Account(ledger.GetUrl()).Main().Put(ledger)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store synthetic transaction ledger: %w", err)
		}
	}

	return partLedger, nil
}

func (x SequencedMessage) loadLedger(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, protocol.SequenceLedger, error) {
	var isAnchor bool
	u := ctx.Executor.Describe.Synthetic()
	isAnchor, err := x.isAnchor(batch, ctx, seq)
	if err != nil {
		return false, nil, errors.UnknownError.Wrap(err)
	}
	if isAnchor {
		u = ctx.Executor.Describe.AnchorPool()
	}

	var ledger protocol.SequenceLedger
	err = batch.Account(u).Main().GetAs(&ledger)
	if err != nil {
		msg := "synthetic"
		if isAnchor {
			msg = "anchor"
		}
		return false, nil, errors.UnknownError.WithFormat("load %s ledger: %w", msg, err)
	}

	return isAnchor, ledger, nil
}
