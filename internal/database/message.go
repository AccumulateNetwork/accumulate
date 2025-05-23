// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func (b *Batch) Message2(hash []byte) *Message {
	return b.Message(*(*[32]byte)(hash))
}

func (m *Message) hash() [32]byte {
	return m.key.Get(1).([32]byte)
}

func (m *Message) Main() values.Value[messaging.Message] {
	return (*messageMain)(m)
}

// messageMain returns the message's main state but falls back to
// batch.Transaction(hash).Main() if the message's main state does not exist.
type messageMain Message

func (m *messageMain) Key() *record.Key              { return (*Message)(m).getMain().Key() }
func (m *messageMain) IsDirty() bool                 { return (*Message)(m).getMain().IsDirty() }
func (m *messageMain) Commit() error                 { return (*Message)(m).getMain().Commit() }
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
		return &messaging.TransactionMessage{
			Transaction: s.Transaction,
		}, nil

	case s.Signature != nil:
		// It's a signature so return a UserSignature
		return &messaging.SignatureMessage{
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
		if errors.Is(err, errors.NotFound) {
			return err // Do not wrap database.NotFoundError
		}
		return errors.UnknownError.Wrap(err)
	}

	err = encoding.SetPtr(u, target)
	return errors.WrongType.Wrap(err)
}

// Resolve implements Record.Resolve.
func (m *messageMain) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return m, nil, nil
	}
	return nil, nil, errors.InternalError.With("bad key for value")
}

func (m *messageMain) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	var err error
	values.Walk(&err, m.main, opts, fn)
	return err
}
