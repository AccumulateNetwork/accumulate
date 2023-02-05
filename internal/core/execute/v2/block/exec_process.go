// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// bundle is a bundle of messages to be processed.
type bundle struct {
	*BlockV2

	// pass indicates that the bundle's Nth pass is currently being processed.
	pass int

	// messages is the bundle of messages.
	messages []messaging.Message

	// additional is an additional bundle of messages that should be processed
	// after this one.
	additional []messaging.Message

	// transactionsToProcess is a list of transactions that have been signed and
	// thus should be processed. It will be removed once authority signatures
	// are implemented. MUST BE ORDERED.
	transactionsToProcess hashSet

	// internal tracks which messages were produced internally (e.g. network
	// account updates).
	internal set[[32]byte]

	// forwarded tracks which messages were forwarded within a forwarding
	// message.
	forwarded set[[32]byte]

	// state tracks transaction state objects.
	state orderedMap[[32]byte, *chain.ProcessTransactionState]

	// produced is other messages produced while processing the bundle.
	produced []*ProducedMessage
}

// Process processes a message bundle.
func (b *BlockV2) Process(messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	var statuses []*protocol.TransactionStatus

	// Make sure every transaction is signed
	err := b.checkForUnsignedTransactions(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Do not check for unsigned transactions when processing additional
	// messages
	var pass int
additional:

	// Put transactions last
	for i, msg := range messages {
		_, ok := unwrapMessageAs[messaging.MessageWithTransaction](msg)
		if !ok {
			continue
		}
		copy(messages[i:], messages[i+1:])
		messages[len(messages)-1] = msg
	}

	// Do this now for the sake of comparing logs
	for _, msg := range messages {
		if fwd, ok := msg.(*internal.ForwardedMessage); ok {
			msg = fwd.Message
		}

		msg, ok := msg.(*messaging.UserTransaction)
		if !ok {
			continue
		}

		fn := b.Executor.logger.Debug
		kv := []interface{}{
			"block", b.Block.Index,
			"type", msg.Transaction.Body.Type(),
			"txn-hash", logging.AsHex(msg.Transaction.GetHash()).Slice(0, 4),
			"principal", msg.Transaction.Header.Principal,
		}
		switch msg.Transaction.Body.Type() {
		case protocol.TransactionTypeDirectoryAnchor,
			protocol.TransactionTypeBlockValidatorAnchor:
			fn = b.Executor.logger.Info
			kv = append(kv, "module", "anchoring")
		}
		if pass > 0 {
			fn("Executing additional", kv...)
		} else {
			fn("Executing transaction", kv...)
		}
	}

	d := new(bundle)
	d.BlockV2 = b
	d.pass = pass
	d.messages = messages
	d.state = orderedMap[[32]byte, *chain.ProcessTransactionState]{cmp: func(u, v [32]byte) int { return bytes.Compare(u[:], v[:]) }}
	d.transactionsToProcess = hashSet{}
	d.internal = set[[32]byte]{}
	d.forwarded = set[[32]byte]{}

	// Process each message
	remote := set[[32]byte]{}
	for _, msg := range messages {
		st, err := d.callMessageExecutor(b.Block.Batch, &MessageContext{bundle: d, message: msg})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Some executors may not produce a status at this stage
		if st != nil {
			statuses = append(statuses, st)
		}
	}

	// Process transactions (MUST BE ORDERED)
	for _, hash := range d.transactionsToProcess {
		remote.Remove(hash)
		st, err := d.executeTransaction(hash)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if st != nil {
			statuses = append(statuses, st)
		}
	}

	// Record remote transactions (remove?)
	for hash := range remote {
		hash := hash // See docs/developer/rangevarref.md
		record := b.Block.Batch.Transaction(hash[:])
		st, err := record.Status().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction status: %w", err)
		}
		if st.Code != 0 {
			continue
		}

		st.Code = errors.Remote
		err = record.Status().Put(st)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store transaction status: %w", err)
		}
	}

	// Produce remote signatures
	err = d.ProcessRemoteSignatures()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Process synthetic transactions generated by the validator
	err = b.Executor.ProduceSynthetic(b.Block.Batch, d.produced)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Update the block state (MUST BE ORDERED)
	_ = d.state.For(func(_ [32]byte, state *chain.ProcessTransactionState) error {
		b.Block.State.MergeTransaction(state)
		return nil
	})

	// Process additional transactions. It would be simpler to do this
	// recursively, but it's possible that could cause a stack overflow.
	if len(d.additional) > 0 {
		pass++
		messages = d.additional
		goto additional
	}

	b.Block.State.Produced += len(d.produced)
	return statuses, nil
}

// checkForUnsignedTransactions returns an error if the message bundle includes
// any unsigned transactions.
func (b *BlockV2) checkForUnsignedTransactions(messages []messaging.Message) error {
	unsigned := set[[32]byte]{}
	for _, msg := range messages {
		if msg, ok := msg.(*messaging.UserTransaction); ok {
			unsigned[msg.ID().Hash()] = struct{}{}
		}
	}
	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.UserSignature:
			delete(unsigned, msg.TxID.Hash())
		case *messaging.ValidatorSignature:
			delete(unsigned, msg.Signature.GetTransactionHash())
		}
	}
	if len(unsigned) > 0 {
		return errors.BadRequest.With("message bundle includes an unsigned transaction")
	}
	return nil
}

// callMessageExecutor finds the executor for the message and calls it.
func (b *bundle) callMessageExecutor(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	// Internal messages are not allowed on the first pass. This is probably
	// unnecessary since internal messages cannot be marshalled, but better safe
	// than sorry.
	if b.pass == 0 && ctx.Type() >= internal.MessageTypeInternal {
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type())), nil
	}

	// Find the appropriate executor
	x, ok := b.Executor.messageExecutors[ctx.Type()]
	if !ok {
		// If the message type is internal, this is almost certainly a bug
		if ctx.Type() >= internal.MessageTypeInternal {
			return nil, errors.InternalError.WithFormat("no executor registered for message type %v", ctx.Type())
		}
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("unsupported message type %v", ctx.Type())), nil
	}

	// Process the message
	st, err := x.Process(batch, ctx)
	err = errors.UnknownError.Wrap(err)
	return st, err
}

// callSignatureExecutor finds the executor for the signature and calls it.
func (b *bundle) callSignatureExecutor(batch *database.Batch, ctx *SignatureContext) (*protocol.TransactionStatus, error) {
	// Find the appropriate executor
	x, ok := b.Executor.signatureExecutors[ctx.Type()]
	if !ok {
		return protocol.NewErrorStatus(ctx.message.ID(), errors.BadRequest.WithFormat("unsupported signature type %v", ctx.Type())), nil
	}

	// Process the message
	st, err := x.Process(batch, ctx)
	err = errors.UnknownError.Wrap(err)
	return st, err
}

// executeTransaction executes a transaction.
func (b *bundle) executeTransaction(hash [32]byte) (*protocol.TransactionStatus, error) {
	batch := b.Block.Batch.Begin(true)
	defer batch.Discard()

	// Load the transaction. Earlier checks should guarantee this never fails.
	var txn messaging.MessageWithTransaction
	err := batch.Message(hash).Main().GetAs(&txn)
	if err != nil {
		return nil, errors.InternalError.WithFormat("load transaction: %w", err)
	}

	status, err := batch.Transaction(hash[:]).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Do not process the transaction if it has already been delivered
	if status.Delivered() {
		return status, nil
	}

	delivery := &chain.Delivery{Transaction: txn.GetTransaction(), Internal: b.internal.Has(hash)}
	if typ := txn.GetTransaction().Body.Type(); typ.IsSynthetic() || typ.IsAnchor() {
		// Load sequence info (nil bundle is a hack)
		delivery.Sequence, err = (*bundle)(nil).getSequence(batch, delivery.Transaction.ID())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load sequence info: %w", err)
		}
	}

	status, state, err := b.Executor.ProcessTransaction(batch, delivery)
	if err != nil {
		return nil, err
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit batch: %w", err)
	}

	kv := []interface{}{
		"block", b.Block.Index,
		"type", txn.GetTransaction().Body.Type(),
		"code", status.Code,
		"txn-hash", logging.AsHex(txn.GetTransaction().GetHash()).Slice(0, 4),
		"principal", txn.GetTransaction().Header.Principal,
	}
	if status.Error != nil {
		kv = append(kv, "error", status.Error)
		if b.pass > 0 {
			b.Executor.logger.Info("Additional transaction failed", kv...)
		} else {
			b.Executor.logger.Info("Transaction failed", kv...)
		}
	} else if status.Pending() {
		if b.pass > 0 {
			b.Executor.logger.Debug("Additional transaction pending", kv...)
		} else {
			b.Executor.logger.Debug("Transaction pending", kv...)
		}
	} else {
		fn := b.Executor.logger.Debug
		switch txn.GetTransaction().Body.Type() {
		case protocol.TransactionTypeDirectoryAnchor,
			protocol.TransactionTypeBlockValidatorAnchor:
			fn = b.Executor.logger.Info
			kv = append(kv, "module", "anchoring")
		}
		if b.pass > 0 {
			fn("Additional transaction succeeded", kv...)
		} else {
			fn("Transaction succeeded", kv...)
		}
	}

	for _, newTxn := range state.ProducedTxns {
		msg := &messaging.UserTransaction{Transaction: newTxn}
		prod := &ProducedMessage{Producer: txn.ID(), Message: msg}
		b.produced = append(b.produced, prod)
	}
	b.additional = append(b.additional, state.AdditionalMessages...)
	b.state.Set(hash, state)
	return status, nil
}
