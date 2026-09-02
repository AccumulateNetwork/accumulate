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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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

	// Move the stream, last: everything this message records is in the batch
	// and about to commit. An advance is block state, not batch state, so it
	// cannot ride the discard — it is only applied once nothing left can fail.
	if ctx.advance != nil {
		err = ctx.advance()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
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
	str, ready, err := x.isReady(batch, ctx, seq)
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

	// Advance the stream — delivered if the transaction succeeded OR failed,
	// pending otherwise. The ledger record itself is written once per stream
	// when the block closes (#4169 step 7); this moves the block's position so
	// the next ask sees it. Deferred to Process, so it lands only once
	// everything this message records has, and not on a path that discards.
	delivered := !st.Pending()
	ctx.advance = func() error {
		return ctx.Block.advanceStream(str, delivered, seq.Number, seq.ID())
	}

	if !st.Delivered() {
		return false, nil
	}

	// Nothing is scheduled here. The stage that ran this message already
	// contains the whole contiguous run behind it (#4169) — draining a staged
	// tail is that stage's walk continuing, not a consequence of this
	// delivery. What used to live here decided, per delivery, whether the
	// successor could run inline or had to wait for the next block; a stage
	// decides that once, for the whole run, before anything executes.
	return true, nil
}

// isReady reports which stream governs the message and whether it is next on
// it. It asks the block's position, never the ledger: the position is read
// once per stream per block and advanced as the block executes, which is
// where the per-message ledger read — and the O(n^2) drain — went (#4169
// step 7).
func (x SequencedMessage) isReady(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (stream, bool, error) {
	// One rule, stated in streamOf (#4169 step 1). The executor's lookup
	// searches the bundle before the database, which is why it is passed in
	// rather than assumed.
	str, err := ctx.Executor.streamFor(seq, func(hash [32]byte) (*protocol.Transaction, error) {
		return ctx.getTransaction(batch, hash)
	})
	if err != nil {
		return stream{}, false, errors.UnknownError.Wrap(err)
	}
	pos, err := ctx.Block.positionOf(str)
	if err != nil {
		return stream{}, false, errors.UnknownError.Wrap(err)
	}

	// If the sequence number is old, mark it already delivered
	typ := "synthetic message"
	if str.kind == streamAnchor {
		typ = "anchor"
	}
	if seq.Number <= pos.delivered {
		return str, false, errors.Delivered.WithFormat("%s has been delivered", typ)
	}

	// If the transaction is out of sequence, mark it pending
	if pos.next() != seq.Number {
		ctx.Executor.logger.Debug("Out of sequence message",
			"hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
			"seq-got", seq.Number,
			"seq-want", pos.next(),
			"source", seq.Source,
			"destination", seq.Destination,
			"type", typ,
		)
		return str, false, nil
	}

	return str, true, nil
}
