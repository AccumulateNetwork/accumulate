// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type committer interface {
	Commit() error
	Discard()
}

func commitOrDiscard(batch committer, err *error) {
	if *err != nil {
		batch.Discard()
		return
	}

	e := batch.Commit()
	*err = errors.UnknownError.Skip(1).Wrap(e)
}

type MessageContext struct {
	*bundle

	message messaging.Message

	statuses   []*protocol.TransactionStatus
	additional []messaging.Message
}

func (m *MessageContext) Type() messaging.MessageType { return m.message.Type() }

func (m *MessageContext) recordErrorStatus(err error) {
	st := protocol.NewErrorStatus(m.message.ID(), err)
	m.statuses = append(m.statuses, st)
}

// childWith constructs a child message context for the given message.
func (m *MessageContext) childWith(msg messaging.Message) *MessageContext {
	n := new(MessageContext)
	n.bundle = m.bundle
	n.message = msg
	return n
}

// callMessageExecutor creates a child context for the given message and calls
// the corresponding message executor.
func (m *MessageContext) callMessageExecutor(batch *ChangeSet, msg messaging.Message) error {
	c := m.childWith(msg)
	err := m.bundle.callMessageExecutor(batch, c)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	m.statuses = append(m.statuses, c.statuses...)
	m.additional = append(m.additional, c.additional...)
	return nil
}

// callMessageValidator creates a child context for the given message and calls
// the corresponding message executor.
func (m *MessageContext) callMessageValidator(batch *ChangeSet, msg messaging.Message) error {
	c := m.childWith(msg)
	err := m.bundle.callMessageValidator(batch, c)
	m.statuses = append(m.statuses, c.statuses...)
	return errors.UnknownError.Wrap(err)
}
