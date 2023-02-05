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

// sigWith constructs a signature context from this message context for the given signature and transaction.
func (m *MessageContext) sigWith(sig protocol.Signature, txn *protocol.Transaction) *SignatureContext {
	s := new(SignatureContext)
	s.MessageContext = m
	s.signature = sig
	s.transaction = txn
	return s
}

// isWithin returns true if the given message type appears somewhere in the
// message chain.
func (m *MessageContext) isWithin(typ messaging.MessageType) bool {
	for m := m; m != nil; m = m.parent {
		if m.message.Type() == typ {
			return true
		}
	}
	return false
}

// getSequence gets the [message.SequencedMessage] cause of the given message, if one exists.
func getSequence(batch *database.Batch, id *url.TxID) (*messaging.SequencedMessage, error) {
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
