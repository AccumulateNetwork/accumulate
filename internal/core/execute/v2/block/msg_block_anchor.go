// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[BlockAnchor](&messageExecutors, messaging.MessageTypeBlockAnchor)
}

// BlockAnchor executes the signature, queuing the transaction for processing
// when appropriate.
type BlockAnchor struct{}

func (x BlockAnchor) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, _, _, _, err := x.check(ctx, batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate the transaction
	_, err = ctx.callMessageValidator(batch, msg.Anchor)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// TODO Validate the signature (but NOT using the user signature executor)
	return nil, nil
}

func (x BlockAnchor) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	// Check if the message has already been processed
	status, err := ctx.checkStatus(batch)
	if err != nil || status.Delivered() {
		return status, err
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	// Process the message
	msg, txn, seq, signer, err := x.check(ctx, batch)
	if err == nil {
		err = x.process(batch, ctx, msg, txn, seq, signer)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x BlockAnchor) process(batch *database.Batch, ctx *MessageContext, msg *messaging.BlockAnchor, txn *protocol.Transaction, seq *messaging.SequencedMessage, signer protocol.Signer2) error {
	// Record the anchor signature
	err := batch.Account(txn.Header.Principal).
		Transaction(txn.ID().Hash()).
		AnchorSignatures().
		Add(msg.Signature)
	if err != nil {
		// A system error occurred
		return errors.UnknownError.Wrap(err)
	}

	ready, err := x.txnIsReady(batch, ctx, txn, seq)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if ready {
		// Process the transaction
		_, err = ctx.callMessageExecutor(batch, seq)
	} else {
		// Mark the message as pending
		_, err = ctx.recordPending(batch, ctx, seq.Message)
	}
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}

// check checks if the message is garbage or not.
func (x BlockAnchor) check(ctx *MessageContext, batch *database.Batch) (*messaging.BlockAnchor, *protocol.Transaction, *messaging.SequencedMessage, protocol.Signer2, error) {
	anchor, ok := ctx.message.(*messaging.BlockAnchor)
	if !ok {
		return nil, nil, nil, nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBlockAnchor, ctx.message.Type())
	}

	if anchor.Signature == nil {
		return nil, nil, nil, nil, errors.BadRequest.With("missing signature")
	}
	if anchor.Anchor == nil {
		return nil, nil, nil, nil, errors.BadRequest.With("missing anchor")
	}
	if anchor.Signature.GetTransactionHash() == ([32]byte{}) {
		return nil, nil, nil, nil, errors.BadRequest.With("missing transaction hash")
	}

	// Verify the anchor is a sequenced anchor transaction
	seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
	if !ok {
		return nil, nil, nil, nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeSequenced, anchor.Anchor.Type())
	}
	txn, ok := seq.Message.(*messaging.TransactionMessage)
	if !ok {
		return nil, nil, nil, nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeTransaction, seq.Message.Type())
	}
	if typ := txn.GetTransaction().Body.Type(); !typ.IsAnchor() {
		return nil, nil, nil, nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", typ, anchor.Type())
	}

	if seq.Source == nil {
		return nil, nil, nil, nil, errors.InternalError.WithFormat("sequence is missing source")
	}

	// Basic validation
	h := seq.Hash()
	if !anchor.Signature.Verify(nil, h[:]) {
		return nil, nil, nil, nil, errors.Unauthenticated.WithFormat("invalid signature")
	}

	// Verify the signer is a validator of this partition
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return nil, nil, nil, nil, errors.BadRequest.WithFormat("signature source is not a partition")
	}

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	signer := ctx.Executor.globals.Active.AsSigner(partition)
	_, _, ok = signer.EntryByKeyHash(anchor.Signature.GetPublicKeyHash())
	if !ok {
		return nil, nil, nil, nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
	}

	return anchor, txn.GetTransaction(), seq, signer, nil
}

func (x BlockAnchor) txnIsReady(batch *database.Batch, ctx *MessageContext, txn *protocol.Transaction, seq *messaging.SequencedMessage) (bool, error) {
	sigs, err := batch.Account(txn.Header.Principal).
		Transaction(txn.ID().Hash()).
		AnchorSignatures().
		Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load anchor signatures: %w", err)
	}

	// Have we received enough signatures?
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return false, errors.BadRequest.WithFormat("source %v is not a partition", seq.Source)
	}
	if uint64(len(sigs)) < ctx.Executor.globals.Active.ValidatorThreshold(partition) {
		return false, nil
	}

	return true, nil
}
