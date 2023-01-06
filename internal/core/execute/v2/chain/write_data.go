// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"bytes"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteData struct{}

var _ PrincipalValidator = (*WriteData)(nil)
var _ SignerValidator = (*WriteData)(nil)

func (WriteData) Type() protocol.TransactionType { return protocol.TransactionTypeWriteData }

func isWriteToLiteDataAccount(batch *database.Batch, transaction *protocol.Transaction) (bool, error) {
	body, ok := transaction.Body.(*protocol.WriteData)
	if !ok {
		return false, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.WriteData), transaction.Body)
	}

	chainId, err := protocol.ParseLiteDataAddress(transaction.Header.Principal)
	if err != nil {
		return false, nil //nolint:nilerr // Not a lite data address
	}

	account, err := batch.Account(transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Found the account, is it a lite data account?
		_, ok := account.(*protocol.LiteDataAccount)
		return ok, nil

	case errors.Is(err, errors.NotFound):
		// Are we creating a lite data account?
		computedChainId := protocol.ComputeLiteDataAccountId(body.Entry)
		return bytes.HasPrefix(computedChainId, chainId), nil

	default:
		// Unknown error
		return false, errors.UnknownError.Wrap(err)
	}
}

func (WriteData) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// WriteData can create a lite data account
	_, err := protocol.ParseLiteDataAddress(transaction.Header.Principal)
	return err == nil
}

// SignerIsAuthorized returns nil if the transaction is writing to a lite data
// account.
func (WriteData) SignerIsAuthorized(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, _ protocol.Signer, _ SignatureValidationMetadata) (fallback bool, err error) {
	lite, err := isWriteToLiteDataAccount(batch, transaction)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	return !lite, nil
}

// TransactionIsReady returns true if the transaction is writing to a lite data
// account.
func (WriteData) TransactionIsReady(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	lite, err := isWriteToLiteDataAccount(batch, transaction)
	if err != nil {
		return false, false, errors.UnknownError.Wrap(err)
	}

	// Writing to a lite data account only requires one signature
	if lite {
		return len(status.Signers) > 0, false, nil
	}

	// Fallback to general authorization
	return false, true, nil
}

func (WriteData) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (WriteData{}).Validate(st, tx)
}

func (WriteData) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteData)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.WriteData), tx.Transaction.Body)
	}

	err := validateDataEntry(st, body.Entry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	//check will return error if there is too much data or no data for the entry
	_, err = protocol.CheckDataEntrySize(body.Entry)
	if err != nil {
		return nil, err
	}

	_, err = protocol.ParseLiteDataAddress(st.OriginUrl)
	if err == nil {
		if body.WriteToState {
			return nil, errors.BadRequest.WithFormat("cannot write data to the state of a lite data account")
		}
		return executeWriteLiteDataAccount(st, body.Entry)
	}

	return executeWriteFullDataAccount(st, body.Entry, body.Scratch, body.WriteToState)
}

func executeWriteFullDataAccount(st *StateManager, entry protocol.DataEntry, scratch bool, writeToState bool) (protocol.TransactionResult, error) {
	if st.Origin == nil {
		return nil, errors.NotFound.WithFormat("%v not found", st.OriginUrl)
	}

	account, ok := st.Origin.(*protocol.DataAccount)
	if !ok {
		return nil, errors.BadRequest.WithFormat("invalid principal: want %v, got %v",
			protocol.AccountTypeDataAccount, st.Origin.Type())
	}

	if writeToState {
		if scratch {
			return nil, errors.BadRequest.WithFormat("writing scratch data to the account state is not permitted")
		}

		account.Entry = entry
		err := st.Update(account)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("store account: %w", err)
		}
	}

	result := new(protocol.WriteDataResult)
	result.EntryHash = *(*[32]byte)(entry.Hash())
	result.AccountID = st.OriginUrl.AccountID()
	result.AccountUrl = st.OriginUrl

	st.UpdateData(st.Origin, result.EntryHash[:], entry)
	return result, nil
}

func validateDataEntry(st *StateManager, entry protocol.DataEntry) error {
	if entry == nil {
		return errors.BadRequest.WithFormat("entry is missing")
	}

	limit := int(st.Globals.Globals.Limits.DataEntryParts)
	switch entry := entry.(type) {
	case *protocol.FactomDataEntryWrapper:
		// Avoid allocating, return a different error message
		if len(entry.ExtIds) > limit-1 {
			return errors.BadRequest.WithFormat("data entry contains too many ext IDs")
		}

	default:
		if len(entry.GetData()) > limit {
			return errors.BadRequest.WithFormat("data entry contains too many parts")
		}
	}

	return nil
}
