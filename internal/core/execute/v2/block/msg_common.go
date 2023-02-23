// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MessageContext is the context in which a message is processed.
type MessageContext struct {
	// bundle is the message bundle being processed.
	*bundle

	// parent is the parent message context, or nil.
	parent *MessageContext

	// message is the Message being processed.
	message messaging.Message

	// additional is additional messages that should be processed after this one.
	additional []messaging.Message

	// produced is other messages produced while processing the message.
	produced []*ProducedMessage
}

func (m *MessageContext) Type() messaging.MessageType { return m.message.Type() }

// childWith constructs a child message context for the given message.
func (m *MessageContext) childWith(msg messaging.Message) *MessageContext {
	n := new(MessageContext)
	n.bundle = m.bundle
	n.message = msg
	n.parent = m
	return n
}

// sigWith constructs a signature context from this message context for the
// given signature and transaction.
func (m *MessageContext) sigWith(sig protocol.Signature, txn *protocol.Transaction) *SignatureContext {
	s := new(SignatureContext)
	s.MessageContext = m
	s.signature = sig
	s.transaction = txn
	return s
}

// txnWith constructs a transaction context from this message context for the
// given transaction.
func (m *MessageContext) txnWith(txn *protocol.Transaction) *TransactionContext {
	t := new(TransactionContext)
	t.MessageContext = m
	t.transaction = txn
	return t
}

// isWithin returns true if the given message type appears somewhere in the
// message chain.
func (m *MessageContext) isWithin(typ ...messaging.MessageType) bool {
	for {
		if m.parent == nil {
			return false
		}
		m = m.parent
		for _, typ := range typ {
			if m.message.Type() == typ {
				return true
			}
		}

		// Break the chain when processing a signature. The signature itself
		// might be synthetic, but that does not make the transaction synthetic.
		//
		// An authority signature necessarily must be executed from within a
		// synthetic context - however the transaction is necessarily a user
		// transaction, and user transactions cannot be executed within a
		// synthetic context. Thus we break the chain so we can execute inline
		// without the transaction executor complaining about a user transaction
		// within a synthetic context.
		switch m.message.Type() {
		case messaging.MessageTypeSignature,
			messaging.MessageTypeCreditPayment:
			return false
		}
	}
}

// getMessageContextAncestor returns the ancestor of the context that is the
// given type.
func getMessageContextAncestor[T any](m *MessageContext) (T, bool) {
	for {
		if m.parent == nil {
			var z T
			return z, false
		}
		m = m.parent
		if x, ok := m.message.(T); ok {
			return x, true
		}

		// For the same reasons as MessageContext.isWithin
		switch m.message.Type() {
		case messaging.MessageTypeSignature,
			messaging.MessageTypeCreditPayment:
			var z T
			return z, false
		}
	}
}

// shouldExecuteTransaction checks if this context is one that is safe to
// execute a transaction within.
//
// This is some seriously questionable logic but I can't think of anything
// better right now.
func (m *MessageContext) shouldExecuteTransaction() bool {
	for {
		// Do not execute a transaction from within a bare transaction message
		if m.parent == nil {
			return false
		}

		// Do not execute a transaction from within a bare sequenced message
		if m.parent.Type() != messaging.MessageTypeSequenced {
			return true
		}
		m = m.parent
	}
}

// queueAdditional queues an additional message for processing after the current
// bundle.
func (m *MessageContext) queueAdditional(msg messaging.Message) {
	m.additional = append(m.additional, msg)
}

// didProduce queues a produced synthetic message for dispatch.
func (m *MessageContext) didProduce(dest *url.URL, msg messaging.Message) {
	m.produced = append(m.produced, &ProducedMessage{
		Producer:    m.message.ID(),
		Destination: dest,
		Message:     msg,
	})
}

// callMessageExecutor creates a child context for the given message and calls
// the corresponding message executor.
func (m *MessageContext) callMessageExecutor(batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	c := m.childWith(msg)
	st, err := m.bundle.callMessageExecutor(batch, c)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	m.additional = append(m.additional, c.additional...)
	m.produced = append(m.produced, c.produced...)
	return st, nil
}

// callMessageValidator creates a child context for the given message and calls
// the corresponding message executor.
func (m *MessageContext) callMessageValidator(batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	c := m.childWith(msg)
	st, err := m.bundle.callMessageValidator(batch, c)
	return st, errors.UnknownError.Wrap(err)
}

// getTransaction loads a transaction from the database or from the message bundle.
func (b *bundle) getTransaction(batch *database.Batch, hash [32]byte) (*protocol.Transaction, error) {
	// Look in the bundle
	for _, msg := range b.messages {
		txn, ok := messaging.UnwrapAs[messaging.MessageWithTransaction](msg)
		if ok &&
			txn.GetTransaction().Body.Type() != protocol.TransactionTypeRemote &&
			txn.Hash() == hash {
			return txn.GetTransaction(), nil
		}
	}

	// Look in the database
	var txn messaging.MessageWithTransaction
	err := batch.Message(hash).Main().GetAs(&txn)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return txn.GetTransaction(), nil
}

// getSignature loads a signature from the database or from the message bundle.
func (b *bundle) getSignature(batch *database.Batch, hash [32]byte) (protocol.Signature, error) {
	// Look in the bundle
	for _, msg := range b.messages {
		msg, ok := messaging.UnwrapAs[messaging.MessageWithSignature](msg)
		if ok && bytes.Equal(msg.GetSignature().Hash(), hash[:]) {
			return msg.GetSignature(), nil
		}
	}

	// Look in the database
	var msg messaging.MessageWithSignature
	err := batch.Message(hash).Main().GetAs(&msg)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return msg.GetSignature(), nil
}

func (b *bundle) recordPending(batch *database.Batch, ctx *MessageContext, msg messaging.Message) (*protocol.TransactionStatus, error) {
	h := msg.Hash()
	ctx.Executor.logger.Debug("Pending sequenced message", "hash", logging.AsHex(h).Slice(0, 4), "module", "synthetic")

	// Store the message
	err := batch.Message(h).Main().Put(msg)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store message: %w", err)
	}

	// Update the status
	status, err := batch.Transaction(h[:]).Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}
	status.TxID = msg.ID()
	status.Code = errors.Pending
	if status.Received == 0 {
		status.Received = ctx.Block.Index
	}
	err = batch.Transaction(h[:]).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store status: %w", err)
	}

	// Add a transaction state
	_, ok := ctx.state.Get(msg.Hash())
	if !ok {
		ctx.state.Set(msg.Hash(), new(chain.ProcessTransactionState))
	}

	return status, nil
}
