// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func (b *Batch) Message2(hash []byte) *Message {
	return b.Message(*(*[32]byte)(hash))
}

func (m *Message) hash() [32]byte {
	return m.key[1].([32]byte)
}

func (m *Message) Main() record.Value[messaging.Message] {
	return (*messageMain)(m)
}

// messageMain returns the message's main state but falls back to
// batch.Transaction(hash).Main() if the message's main state does not exist.
type messageMain Message

func (m *messageMain) IsDirty() bool                 { return (*Message)(m).getMain().IsDirty() }
func (m *messageMain) Commit() error                 { return (*Message)(m).getMain().Commit() }
func (m *messageMain) Key(n int) any                 { return (*Message)(m).getMain().Key(n) }
func (m *messageMain) Put(v messaging.Message) error { return (*Message)(m).getMain().Put(v) }

func (m *messageMain) Get() (messaging.Message, error) {
	// Get the message's main state, if it exists
	msg, err := (*Message)(m).getMain().Get()
	if err == nil || !errors.Is(err, errors.NotFound) {
		return msg, err
	}
	msgErr := err

	// Get the transaction's main state, if it exists
	s, err := m.parent.getTransaction((*Message)(m).hash()).Main().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		// Return the original error
		return nil, msgErr

	case err != nil:
		return nil, err

	case s.Transaction != nil:
		// It's a transaction so return a UserTransaction
		return &messaging.UserTransaction{
			Transaction: s.Transaction,
		}, nil

	case s.Signature != nil:
		// It's a signature so return a UserSignature
		return &messaging.UserSignature{
			Signature: s.Signature,
			TxID:      s.Txid,
		}, nil

	default:
		// Return not found
		return nil, msgErr
	}
}

// GetAs loads the value, coerces it to the target type, and assigns it to the
// target. The target must be a non-nil pointer to T.
func (m *messageMain) GetAs(target interface{}) error {
	u, err := m.Get()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = encoding.SetPtr(u, target)
	return errors.WrongType.Wrap(err)
}

// Resolve implements Record.Resolve.
func (m *messageMain) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return m, nil, nil
	}
	return nil, nil, errors.InternalError.With("bad key for value")
}
