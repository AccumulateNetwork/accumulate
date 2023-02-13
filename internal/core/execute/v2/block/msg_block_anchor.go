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
	_, _, _, _, err := x.check(ctx, batch)
	return nil, errors.UnknownError.Wrap(err)
}

func (x BlockAnchor) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// If the message has already been processed, return its recorded status
	status, err := batch.Transaction2(ctx.message.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	if status.Delivered() {
		return status, nil
	}

	// Add a transaction state to ensure the block gets recorded
	ctx.state.Set(ctx.message.Hash(), new(chain.ProcessTransactionState))

	batch = batch.Begin(true)
	defer batch.Discard()

	// Process the message
	status = new(protocol.TransactionStatus)
	status.Received = ctx.Block.Index
	status.TxID = ctx.message.ID()

	msg, txn, seq, signer, err := x.check(ctx, batch)
	if err == nil {
		err = x.process(batch, ctx, msg, txn, seq, signer)
	}

	// Update the status
	switch {
	case err == nil:
		status.Code = errors.Delivered

	case errors.Code(err).IsClientError():
		status.Set(err)

	default:
		return nil, errors.UnknownError.Wrap(err)
	}

	// Record the message
	err = batch.Message(ctx.message.Hash()).Main().Put(ctx.message)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	if status.Delivered() {
		err = batch.Message(ctx.message.Hash()).Produced().Add(txn.ID())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("add cause: %w", err)
		}

		err = batch.Message2(txn.GetHash()).Cause().Add(ctx.message.ID())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("add cause: %w", err)
		}
	}

	// Record the status
	err = batch.Transaction2(ctx.message.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x BlockAnchor) process(batch *database.Batch, ctx *MessageContext, msg *messaging.BlockAnchor, txn *protocol.Transaction, seq *messaging.SequencedMessage, signer protocol.Signer2) error {
	// Process the signature (update the transaction status)
	err := x.processSignature(ctx, batch, msg, txn, signer)
	if err != nil {
		// A system error occurred
		return errors.UnknownError.Wrap(err)
	}

	ready, err := x.txnIsReady(batch, ctx, seq)
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
	txn, ok := seq.Message.(*messaging.UserTransaction)
	if !ok {
		return nil, nil, nil, nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeUserTransaction, seq.Message.Type())
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
		return nil, nil, nil, nil, errors.BadRequest.WithFormat("invalid signature")
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

func (x BlockAnchor) processSignature(ctx *MessageContext, batch *database.Batch, sig *messaging.BlockAnchor, txn *protocol.Transaction, signer protocol.Signer2) error {
	// Add the anchor signer to the transaction status
	if txn.Body.Type().IsAnchor() {
		txst, err := batch.Transaction(txn.GetHash()).Status().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load transaction status: %w", err)
		}
		txst.AddAnchorSigner(sig.Signature)
		err = batch.Transaction(txn.GetHash()).Status().Put(txst)
		if err != nil {
			return errors.UnknownError.WithFormat("store transaction status: %w", err)
		}
	}

	// Add the signature to the transaction's signature set
	sigSet, err := batch.Transaction(txn.GetHash()).SignaturesForSigner(signer)
	if err != nil {
		return errors.UnknownError.WithFormat("load signatures: %w", err)
	}

	index, _, _ := signer.EntryByKeyHash(sig.Signature.GetPublicKeyHash())
	_, err = sigSet.Add(uint64(index), sig.Signature)
	if err != nil {
		return errors.UnknownError.WithFormat("store signature: %w", err)
	}

	return nil
}

func (x BlockAnchor) txnIsReady(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
	h := seq.Message.ID().Hash()
	status, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// Have we received enough signatures?
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return false, errors.BadRequest.WithFormat("source %v is not a partition", seq.Source)
	}
	if uint64(len(status.AnchorSigners)) < ctx.Executor.globals.Active.ValidatorThreshold(partition) {
		return false, nil
	}

	return true, nil
}
