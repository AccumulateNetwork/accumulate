// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
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

// blockAnchorContext collects all the bits of data needed to process a block anchor.
type blockAnchorContext struct {
	*TransactionContext

	sequenced *messaging.SequencedMessage

	blockAnchor *messaging.BlockAnchor

	signer protocol.Signer2
}

func (x BlockAnchor) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	ctx2, err := x.check(ctx, batch)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Validate the transaction
	_, err = ctx.callMessageValidator(batch, ctx2.blockAnchor.Anchor)
	return nil, errors.UnknownError.Wrap(err)
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
	ctx2, err := x.check(ctx, batch)
	if err == nil {
		err = x.process(batch, ctx2)
	}

	// Record the message and its status
	err = ctx.recordMessageAndStatus(batch, status, errors.Delivered, err)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return status, nil
}

func (x BlockAnchor) process(batch *database.Batch, ctx *blockAnchorContext) error {
	// Record the anchor signature
	err := batch.Account(ctx.effectivePrincipal()).
		Transaction(ctx.transaction.ID().Hash()).
		ValidatorSignatures().
		Add(ctx.blockAnchor.Signature)
	if err != nil {
		// A system error occurred
		return errors.UnknownError.Wrap(err)
	}

	// Add the signature to the signature chain
	err = batch.Account(ctx.effectivePrincipal()).
		Transaction(ctx.transaction.ID().Hash()).
		RecordHistory(ctx.message)
	if err != nil {
		return errors.UnknownError.WithFormat("record history: %w", err)
	}

	ready, err := x.txnIsReady(batch, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if !ready {
		// Mark the message as pending
		_, err = ctx.childWith(ctx.sequenced.Message).recordPending(batch)
		return errors.UnknownError.Wrap(err)
	}

	// Process the transaction
	_, err = ctx.callMessageExecutor(batch, ctx.sequenced)
	return errors.UnknownError.Wrap(err)
}

// check checks if the message is garbage or not.
func (x BlockAnchor) check(ctx *MessageContext, batch *database.Batch) (*blockAnchorContext, error) {
	anchor, ok := ctx.message.(*messaging.BlockAnchor)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBlockAnchor, ctx.message.Type())
	}

	if anchor.Signature == nil {
		return nil, errors.BadRequest.With("missing signature")
	}
	if anchor.Anchor == nil {
		return nil, errors.BadRequest.With("missing anchor")
	}
	if anchor.Signature.GetTransactionHash() == ([32]byte{}) {
		return nil, errors.BadRequest.With("missing transaction hash")
	}

	// Verify the anchor is a sequenced anchor transaction
	seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeSequenced, anchor.Anchor.Type())
	}
	txnMsg, ok := seq.Message.(*messaging.TransactionMessage)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid anchor: expected %v, got %v", messaging.MessageTypeTransaction, seq.Message.Type())
	}

	// Resolve placeholders
	txn := txnMsg.Transaction
	signed := seq.Hash()
	if txn.Body.Type() == protocol.TransactionTypeRemote && ctx.GetActiveGlobals().ExecutorVersion.V2BaikonurEnabled() {
		var err error
		txn, err = ctx.getTransaction(batch, txn.ID().Hash())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}

		// Recalculate the hash with the full transaction
		seq2 := seq.Copy()
		seq2.Message = &messaging.TransactionMessage{Transaction: txn}
		signed = seq2.Hash()
	}

	// Verify the transaction is an anchor
	if !txn.Body.Type().IsAnchor() {
		return nil, errors.BadRequest.WithFormat("cannot sign a %v transaction with a %v message", txn.Body.Type(), anchor.Type())
	}

	if seq.Source == nil {
		return nil, errors.InternalError.WithFormat("sequence is missing source")
	}

	// Basic validation
	if !anchor.Signature.Verify(nil, signed[:]) {
		return nil, errors.Unauthenticated.WithFormat("invalid signature")
	}

	// Verify the signer is a validator of this partition
	partition, ok := protocol.ParsePartitionUrl(seq.Source)
	if !ok {
		return nil, errors.BadRequest.WithFormat("signature source is not a partition")
	}

	// TODO: Consider checking the version. However this can get messy because
	// it takes some time for changes to propagate, so we'd need an activation
	// height or something.

	signer := core.AnchorSigner(&ctx.Executor.globals.Active, partition)
	_, _, ok = signer.EntryByKeyHash(anchor.Signature.GetPublicKeyHash())
	if !ok {
		return nil, errors.Unauthorized.WithFormat("key is not an active validator for %s", partition)
	}

	return &blockAnchorContext{
		TransactionContext: ctx.txnWith(txn),
		sequenced:          seq,
		blockAnchor:        anchor,
		signer:             signer,
	}, nil
}

func (x BlockAnchor) txnIsReady(batch *database.Batch, ctx *blockAnchorContext) (bool, error) {
	sigs, err := batch.Account(ctx.effectivePrincipal()).
		Transaction(ctx.transaction.ID().Hash()).
		ValidatorSignatures().
		Get()
	if err != nil {
		return false, errors.UnknownError.WithFormat("load anchor signatures: %w", err)
	}

	// Have we received enough signatures?
	partition, ok := protocol.ParsePartitionUrl(ctx.sequenced.Source)
	if !ok {
		return false, errors.BadRequest.WithFormat("source %v is not a partition", ctx.sequenced.Source)
	}
	if uint64(len(sigs)) < ctx.Executor.globals.Active.ValidatorThreshold(partition) {
		return false, nil
	}

	return true, nil
}
