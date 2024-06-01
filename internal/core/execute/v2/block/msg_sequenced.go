// Copyright 2024 The Accumulate Authors
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

	// Queue the next transaction in the sequence
	next, ok := ledger.Get(seq.Number + 1)
	if ok {
		ctx.queueAdditional(&internal.MessageIsReady{TxID: next})
	}

	return true, nil
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
		ctx.Executor.logger.Info("Out of sequence message",
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
