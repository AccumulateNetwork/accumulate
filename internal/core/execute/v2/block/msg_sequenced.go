// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
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
type SequencedMessage struct{ UserTransaction }

func (x SequencedMessage) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	seq, ok := ctx.message.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSequenced, ctx.message.Type())
	}

	// Basic validation
	if seq.Message == nil {
		return protocol.NewErrorStatus(seq.ID(), errors.BadRequest.With("missing message")), nil
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
		return protocol.NewErrorStatus(seq.ID(), errors.BadRequest.WithFormat("invalid synthetic transaction: missing %s", strings.Join(missing, ", "))), nil
	}

	if !ctx.Executor.Describe.NodeUrl().Equal(seq.Destination) {
		return protocol.NewErrorStatus(seq.ID(), errors.BadRequest.WithFormat("invalid destination: expected %v, got %v", ctx.Executor.Describe.NodeUrl(), seq.Destination)), nil
	}

	// Sequenced messages must either be synthetic or anchors
	if !ctx.isWithin(messaging.MessageTypeSynthetic, internal.MessageTypeMessageIsReady) {
		if txn, ok := seq.Message.(*messaging.UserTransaction); !ok || !txn.Transaction.Body.Type().IsAnchor() {
			return protocol.NewErrorStatus(seq.ID(), errors.BadRequest.WithFormat("invalid payload for sequenced message")), nil
		}
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	// Load the transaction
	if txn, ok := seq.Message.(*messaging.UserTransaction); ok {
		_, err := x.resolveTransaction(batch, txn)
		if err != nil {
			if err, ok := err.(*errors.Error); ok && err.Code.IsClientError() {
				return protocol.NewErrorStatus(txn.ID(), err), nil
			}
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Check the status
	h := seq.Message.Hash()
	status, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// If the message has already been processed, return its recorded status
	if status.Delivered() {
		return status, nil
	}

	// Record the message and it's cause/produced relation
	err = batch.Message(seq.Hash()).Main().Put(seq)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	err = batch.Message(seq.Hash()).Produced().Add(seq.Message.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message produced: %w", err)
	}

	err = batch.Message(seq.Message.Hash()).Cause().Add(seq.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message cause: %w", err)
	}

	// Check if the message is ready to process
	ready, err := x.isReady(batch, ctx, seq)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	var st *protocol.TransactionStatus
	if ready {
		// Process the message within
		st, err = ctx.callMessageExecutor(batch, ctx.childWith(seq.Message))
	} else {
		// Mark the message as pending
		st, err = x.recordPending(batch, ctx, seq)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if st == nil {
		err = batch.Commit()
		return nil, errors.UnknownError.Wrap(err)
	}

	// FIXME For anchors the ledger should not be updated until the anchor has
	// enough signatures

	// Update the ledger
	ledger, err := x.updateLedger(batch, ctx, seq, st.Pending())
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Queue the next transaction in the sequence
	if st.Delivered() {
		next, ok := ledger.Get(seq.Number + 1)
		if ok {
			ctx.additional = append(ctx.additional, &internal.MessageIsReady{TxID: next})
		}
	}

	return st, nil
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

func (x SequencedMessage) recordPending(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (*protocol.TransactionStatus, error) {
	h := seq.Message.Hash()
	ctx.Executor.logger.Debug("Pending sequenced message", "hash", logging.AsHex(h).Slice(0, 4), "module", "synthetic")

	// Store the message
	err := batch.Message(h).Main().Put(seq.Message)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	// Update the status
	status, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	status.TxID = seq.Message.ID()
	status.Code = errors.Pending
	if status.Received == 0 {
		status.Received = ctx.Block.Index
	}
	err = batch.Transaction(h[:]).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	// Add a transaction state
	_, ok := ctx.state.Get(seq.Message.Hash())
	if !ok {
		ctx.state.Set(seq.Message.Hash(), new(chain.ProcessTransactionState))
	}

	return status, nil
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
		err = batch.Account(ledger.GetUrl()).PutState(ledger)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store synthetic transaction ledger: %w", err)
		}
	}

	return partLedger, nil
}

func (SequencedMessage) loadLedger(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, protocol.SequenceLedger, error) {
	var isAnchor bool
	u := ctx.Executor.Describe.Synthetic()
	if txn, ok := seq.Message.(*messaging.UserTransaction); ok && txn.Transaction.Body.Type().IsAnchor() {
		isAnchor = true
		u = ctx.Executor.Describe.AnchorPool()
	}

	var ledger protocol.SequenceLedger
	err := batch.Account(u).Main().GetAs(&ledger)
	if err != nil {
		msg := "synthetic"
		if isAnchor {
			msg = "anchor"
		}
		return false, nil, errors.UnknownError.WithFormat("load %s ledger: %w", msg, err)
	}

	return isAnchor, ledger, nil
}
