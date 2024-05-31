// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticWriteData struct{}

var _ PrincipalValidator = (*SyntheticWriteData)(nil)

func (SyntheticWriteData) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticWriteData
}

func (SyntheticWriteData) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// SyntheticWriteData can create a lite data account
	_, err := protocol.ParseLiteDataAddress(transaction.Header.Principal)
	return err == nil
}

func (x SyntheticWriteData) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
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

func (SyntheticWriteData) check(st *StateManager, tx *Delivery) (*protocol.SyntheticWriteData, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticWriteData)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticWriteData), tx.Transaction.Body)
	}

	if body.Entry == nil {
		return nil, errors.BadRequest.WithFormat("entry is nil")
	}

	return body, nil
}

func (x SyntheticWriteData) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	err = validateDataEntry(st, body.Entry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return executeWriteLiteDataAccount(st, body.Entry)
}

func executeWriteLiteDataAccount(st *StateManager, entry protocol.DataEntry) (protocol.TransactionResult, error) {
	var account protocol.Account
	result := new(protocol.WriteDataResult)
	if st.Origin != nil {

		switch origin := st.Origin.(type) {
		case *protocol.LiteDataAccount:
			liteDataAccountId, err := protocol.ParseLiteDataAddress(st.OriginUrl)
			if err != nil {
				return nil, err
			}

			account = origin
			result.EntryHash = *(*[32]byte)(entry.Hash())
			result.AccountID = liteDataAccountId
			result.AccountUrl = st.OriginUrl
		default:
			return nil, fmt.Errorf("invalid principal: want chain type %v, got %v",
				protocol.AccountTypeLiteDataAccount, origin.Type())
		}
	} else if _, err := protocol.ParseLiteDataAddress(st.OriginUrl); err != nil {
		return nil, fmt.Errorf("invalid lite data URL %s: %v", st.OriginUrl.String(), err)
	} else {
		// Address is lite, but the lite data account doesn't exist, so create one
		liteDataAccountId := protocol.ComputeLiteDataAccountId(entry)
		u, err := protocol.LiteDataAddress(liteDataAccountId)
		if err != nil {
			return nil, err
		}

		//the computed data account chain id must match the origin url.
		if !st.OriginUrl.Equal(u) {
			return nil, fmt.Errorf("first entry doesnt match chain id: want %v, got %v", u, st.OriginUrl)
		}

		lite := new(protocol.LiteDataAccount)
		lite.Url = u

		account = lite
		result.EntryHash = *(*[32]byte)(entry.Hash())
		result.AccountID = liteDataAccountId
		result.AccountUrl = u
	}

	st.UpdateData(account, result.EntryHash[:], entry, false)

	return result, nil
}
