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

	// state tracks transaction state objects.
	state orderedMap[[32]byte, *chain.ProcessTransactionState]

	// produced is other messages produced while processing the bundle.
	produced []*ProducedMessage
}

// Process processes a message bundle.
func (b *BlockV2) Process(messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	var statuses []*protocol.TransactionStatus

	// Make sure every transaction is signed
	err := checkForUnsignedTransactions(messages)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Do not check for unsigned transactions when processing additional
	// messages
	var pass int
additional:

	// Do this now for the sake of comparing logs
	for _, msg := range messages {
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

	// Set up the bundle
	d := new(bundle)
	d.BlockV2 = b
	d.pass = pass
	d.messages = messages
	d.state = orderedMap[[32]byte, *chain.ProcessTransactionState]{cmp: func(u, v [32]byte) int { return bytes.Compare(u[:], v[:]) }}

	// Process each message
	for _, msg := range messages {
		ctx := &MessageContext{bundle: d, message: msg}
		st, err := d.callMessageExecutor(b.Block.Batch, ctx)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Some executors may not produce a status at this stage
		if st != nil {
			statuses = append(statuses, st)
		}

		d.additional = append(d.additional, ctx.additional...)
		d.produced = append(d.produced, ctx.produced...)
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
func checkForUnsignedTransactions(messages []messaging.Message) error {
	unsigned := set[[32]byte]{}
	for _, msg := range messages {
		if msg, ok := msg.(*messaging.UserTransaction); ok {
			unsigned[msg.ID().Hash()] = struct{}{}
		}
	}
	for _, msg := range messages {
	again:
		switch m := msg.(type) {
		case messaging.MessageForTransaction:
			delete(unsigned, m.GetTxID().Hash())
		case interface{ Unwrap() messaging.Message }:
			msg = m.Unwrap()
			goto again
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
