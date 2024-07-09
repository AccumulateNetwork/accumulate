// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteData struct{}

var _ SignerCanSignValidator = (*WriteData)(nil)
var _ PrincipalValidator = (*WriteData)(nil)
var _ SignerValidator = (*WriteData)(nil)

func (WriteData) Type() protocol.TransactionType { return protocol.TransactionTypeWriteData }

func isWriteToLiteDataAccount(transaction *protocol.Transaction) bool {
	_, err := protocol.ParseLiteDataAddress(transaction.Header.Principal)
	return err == nil
}

func (WriteData) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	// WriteData can create a lite data account
	return isWriteToLiteDataAccount(transaction)
}

// SignerCanSign returns nil if the transaction is writing to a lite data
// account.
func (WriteData) SignerCanSign(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) (fallback bool, err error) {
	// TODO If WriteData is added as a blacklist option, consider checking it
	// here
	lite := isWriteToLiteDataAccount(transaction)
	return !lite, nil
}

// AuthorityIsAccepted returns nil if the transaction is writing to a lite data
// account.
func (WriteData) AuthorityIsAccepted(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, _ *protocol.AuthoritySignature) (fallback bool, err error) {
	lite := isWriteToLiteDataAccount(transaction)
	return !lite, nil
}

// TransactionIsReady returns true if the transaction is writing to a lite data
// account.
func (WriteData) TransactionIsReady(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	// Fallback to general authorization if not lite
	lite := isWriteToLiteDataAccount(transaction)
	if !lite {
		return false, true, nil
	}

	// Writing to a lite data account only requires one signature
	votes, err := batch.Account(transaction.Header.Principal).
		Transaction(transaction.ID().Hash()).
		Votes().
		Get()
	if err != nil {
		return false, false, errors.UnknownError.WithFormat("load voters: %w", err)
	}
	return len(votes) > 0, false, nil
}

func (x WriteData) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
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

func (WriteData) check(st *StateManager, tx *Delivery) (*protocol.WriteData, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteData)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid payload: want %T, got %T", new(protocol.WriteData), tx.Transaction.Body)
	}

	if body.Entry == nil {
		return nil, errors.BadRequest.WithFormat("entry is nil")
	}
	if !entryIsAccepted(st, body.Entry) {
		return nil, errors.BadRequest.WithFormat("%v data entries are not accepted", body.Entry.Type())
	}

	//check will return error if there is too much data or no data for the entry
	_, err := protocol.CheckDataEntrySize(body.Entry)
	if err != nil {
		return nil, err
	}

	_, err = protocol.ParseLiteDataAddress(st.OriginUrl)
	isLite := err == nil
	if err == nil {
		if body.WriteToState {
			return nil, errors.BadRequest.WithFormat("cannot write data to the state of a lite data account")
		}
		return body, nil
	}

	if body.WriteToState {
		if isLite {
			return nil, errors.BadRequest.WithFormat("cannot write data to the state of a lite data account")
		}
		if body.Scratch {
			return nil, errors.BadRequest.WithFormat("writing scratch data to the account state is not permitted")
		}
	}

	return body, nil
}

func (x WriteData) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	err = validateDataEntry(st, body.Entry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	_, err = protocol.ParseLiteDataAddress(st.OriginUrl)
	if err == nil {
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

	if !st.Globals.ExecutorVersion.V2VandenbergEnabled() {
		// There was an error in previous versions that needs to be preserved to
		// avoid consensus failure
		scratch = false
	}

	st.UpdateData(st.Origin, result.EntryHash[:], entry, scratch)
	return result, nil
}

func entryIsAccepted(st *StateManager, entry protocol.DataEntry) bool {
	switch entry.(type) {
	case *protocol.FactomDataEntryWrapper:
		// Factom entries are accepted
		return true

	case *protocol.AccumulateDataEntry:
		// Accumulate entries are not accepted after v1-doubleHashEntries
		return false

	case *protocol.DoubleHashDataEntry:
		// Double hash entries are not accepted before v1-doubleHashEntries
		return true
	}

	return false
}

func validateDataEntry(st *StateManager, entry protocol.DataEntry) error {
	if entry == nil {
		return errors.BadRequest.WithFormat("entry is missing")
	}
	if !entryIsAccepted(st, entry) {
		return errors.BadRequest.WithFormat("%v data entries are not accepted", entry.Type())
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
