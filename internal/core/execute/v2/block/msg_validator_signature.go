// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[ValidatorSignature](&messageExecutors, messaging.MessageTypeValidatorSignature)
}

// ValidatorSignature executes the signature, queuing the transaction for processing
// when appropriate.
type ValidatorSignature struct{}

func (x ValidatorSignature) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	sig, ok := ctx.message.(*messaging.ValidatorSignature)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeValidatorSignature, ctx.message.Type())
	}

	batch = batch.Begin(true)
	defer batch.Discard()

	status, err := batch.Transaction(sig.Signature.Hash()).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	// If the signature has already been processed, return the stored status
	if status.Code != 0 {
		return status, nil //nolint:nilerr // False positive
	}

	status.TxID = sig.ID()
	status.Received = ctx.Block.Index

	// Check the message for basic validity
	txn, signer, err := x.check(ctx, batch, sig)
	var err2 *errors.Error
	switch {
	case err == nil:
		// Process the signature (update the transaction status)
		err = x.process(ctx, batch, sig, txn, signer)
		if err != nil {
			// A system error occurred
			return nil, errors.UnknownError.Wrap(err)
		}

		status.Code = errors.Delivered

	case errors.As(err, &err2) && err2.Code.IsClientError():
		// Record the error
		status.Set(err)

	default:
		// A system error occurred
		return nil, errors.UnknownError.Wrap(err)
	}

	// Once a signature has been included in the block, record the signature and
	// its status not matter what, unless there is a system error
	err = batch.Message2(sig.Signature.Hash()).Main().Put(sig)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store signature: %w", err)
	}

	err = batch.Transaction(sig.Signature.Hash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	if !status.Failed() {
		// Update the block state
		ctx.Block.State.MergeSignature(&ProcessSignatureState{})

		// Get the sequence message
		seq, err := ctx.getSequence(batch, txn.ID())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load sequence: %w", err)
		}

		// Process the transaction
		st, err := ctx.callMessageExecutor(batch, ctx.childWith(seq))
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		// TODO Do something with the status
		_ = st
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

// check checks if the message is garbage or not.
func (x ValidatorSignature) check(ctx *MessageContext, batch *database.Batch, sig *messaging.ValidatorSignature) (*protocol.Transaction, protocol.Signer2, error) {
	if sig.Signature == nil {
		return nil, nil, errors.BadRequest.With("missing signature")
	}
	if sig.Source == nil {
		return nil, nil, errors.BadRequest.With("missing source")
	}
	if sig.Signature.GetTransactionHash() == ([32]byte{}) {
		return nil, nil, errors.BadRequest.With("missing transaction hash")
	}

	// Basic validation
	h := sig.Signature.GetTransactionHash()
	if !sig.Signature.Verify(nil, h[:]) {
		return nil, nil, errors.BadRequest.WithFormat("invalid signature")
	}

	// Load the transaction
	txn, err := ctx.getTransaction(batch, h)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load transaction: %w", err)
	}

	// Load the sequence
	seq, err := ctx.getSequence(batch, txn.ID())
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	// A validator signature message is only allowed for anchors
	if typ := txn.Body.Type(); !typ.IsAnchor() {
		return nil, nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", typ, sig.Type())
	}

	// Sanity check - this should not happen because the transaction should have
	// been rejected
	if seq.Source == nil {
		return nil, nil, errors.InternalError.WithFormat("transaction is missing source")
	}

	// Verify the sources match
	if !seq.Source.Equal(sig.Source) {
		return nil, nil, errors.BadRequest.WithFormat("source does not match: message has %v, transaction has %v", sig.Source, seq.Source)
	}

	partition, ok := protocol.ParsePartitionUrl(sig.Source)
	if !ok {
		return nil, nil, errors.BadRequest.WithFormat("signature source is not a partition")
	}

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	signer := ctx.Executor.globals.Active.AsSigner(partition)
	_, _, ok = signer.EntryByKeyHash(sig.Signature.GetPublicKeyHash())
	if !ok {
		return nil, nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
	}

	return txn, signer, nil
}

func (x ValidatorSignature) process(ctx *MessageContext, batch *database.Batch, sig *messaging.ValidatorSignature, txn *protocol.Transaction, signer protocol.Signer2) error {
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
