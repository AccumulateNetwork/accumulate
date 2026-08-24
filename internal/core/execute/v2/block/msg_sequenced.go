// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

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
