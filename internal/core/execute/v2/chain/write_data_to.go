// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteDataTo struct{}

func (WriteDataTo) Type() protocol.TransactionType { return protocol.TransactionTypeWriteDataTo }

func (x WriteDataTo) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	result := new(protocol.WriteDataResult)
	result.EntryHash = *(*[32]byte)(body.Entry.Hash())
	result.AccountID = tx.Transaction.Header.Principal.AccountID()
	result.AccountUrl = tx.Transaction.Header.Principal
	return result, nil
}

func (WriteDataTo) check(st *StateManager, tx *Delivery) (*protocol.WriteDataTo, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteDataTo)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.WriteDataTo), tx.Transaction.Body)
	}

	if body.Entry == nil {
		return nil, errors.BadRequest.WithFormat("entry is nil")
	}

	if body.Recipient == nil {
		return nil, errors.BadRequest.WithFormat("recipient is missing")
	}

	err := validateDataEntry(st, body.Entry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if _, err := protocol.ParseLiteDataAddress(body.Recipient); err != nil {
		return nil, errors.BadRequest.WithFormat("only writes to lite data accounts supported: %s: %v", body.Recipient, err)
	}

	return body, nil
}

func (x WriteDataTo) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	writeThis := new(protocol.SyntheticWriteData)
	writeThis.Entry = body.Entry

	st.Submit(body.Recipient, writeThis)

	return nil, nil
}
