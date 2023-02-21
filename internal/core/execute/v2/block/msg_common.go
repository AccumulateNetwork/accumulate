// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MessageContext is the context in which a message is executed.
type MessageContext struct {
	*bundle
	message messaging.Message
	parent  *MessageContext
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
	for m := m; m != nil; m = m.parent {
		for _, typ := range typ {
			if m.message.Type() == typ {
				return true
			}
		}
	}
	return false
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

// getSequence gets the [message.SequencedMessage] cause of the given message, if one exists.
func (b *bundle) getSequence(batch *database.Batch, id *url.TxID) (*messaging.SequencedMessage, error) {
	// Look in the bundle
	if b != nil {
		for _, msg := range b.messages {
			seq, ok := unwrapMessageAs[*messaging.SequencedMessage](msg)
			if ok && seq.Message.ID().Hash() == id.Hash() {
				return seq, nil
			}
		}
	}

	// Look in the database
	causes, err := batch.Message(id.Hash()).Cause().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load causes: %w", err)
	}

	for _, id := range causes {
		msg, err := batch.Message(id.Hash()).Main().Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load message: %w", err)
		}

		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			return seq, nil
		}
	}

	return nil, errors.NotFound.WithFormat("no cause of %v is a sequenced message", id)
}

// getTransaction loads a transaction from the database or from the message bundle.
func (b *bundle) getTransaction(batch *database.Batch, hash [32]byte) (*protocol.Transaction, error) {
	// Look in the bundle
	for _, msg := range b.messages {
		txn, ok := unwrapMessageAs[messaging.MessageWithTransaction](msg)
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
