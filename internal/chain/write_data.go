package chain

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteData struct{}

var _ SignerValidator = (*WriteData)(nil)

func (WriteData) Type() protocol.TransactionType { return protocol.TransactionTypeWriteData }

func isWriteToLiteDataAccount(batch *database.Batch, transaction *protocol.Transaction) (bool, error) {
	body, ok := transaction.Body.(*protocol.WriteData)
	if !ok {
		return false, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.WriteData), transaction.Body)
	}

	chainId, err := protocol.ParseLiteDataAddress(transaction.Header.Principal)
	if err != nil {
		return false, nil // Not a lite data address
	}

	account, err := batch.Account(transaction.Header.Principal).GetState()
	switch {
	case err == nil:
		// Found the account, is it a lite data account?
		_, ok := account.(*protocol.LiteDataAccount)
		return ok, nil

	case errors.Is(err, errors.StatusNotFound):
		// Are we creating a lite data account?
		computedChainId := protocol.ComputeLiteDataAccountId(&body.Entry)
		return bytes.HasPrefix(computedChainId, chainId), nil

	default:
		// Unknown error
		return false, errors.Wrap(errors.StatusUnknown, err)
	}
}

// SignerIsAuthorized returns nil if the transaction is writing to a lite data
// account.
func (WriteData) SignerIsAuthorized(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, _ *url.URL, _ protocol.Signer) (fallback bool, err error) {
	lite, err := isWriteToLiteDataAccount(batch, transaction)
	if err != nil {
		return false, errors.Wrap(errors.StatusUnknown, err)
	}

	return !lite, nil
}

// TransactionIsReady returns true if the transaction is writing to a lite data
// account.
func (WriteData) TransactionIsReady(_ AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	lite, err := isWriteToLiteDataAccount(batch, transaction)
	if err != nil {
		return false, false, errors.Wrap(errors.StatusUnknown, err)
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
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.WriteData), tx.Transaction.Body)
	}

	//check will return error if there is too much data or no data for the entry
	_, err := body.Entry.CheckSize()
	if err != nil {
		return nil, err
	}

	cause := *(*[32]byte)(tx.Transaction.GetHash())
	_, err = protocol.ParseLiteDataAddress(st.OriginUrl)
	if err == nil {
		return executeWriteLiteDataAccount(st, tx.Transaction, &body.Entry, cause, body.Scratch)
	}

	return executeWriteFullDataAccount(st, tx.Transaction, &body.Entry, cause, body.Scratch)
}

func executeWriteFullDataAccount(st *StateManager, txn *protocol.Transaction, entry *protocol.DataEntry, cause [32]byte, scratch bool) (protocol.TransactionResult, error) {
	if st.Origin == nil {
		return nil, errors.NotFound("%v not found", txn.Header.Principal)
	}
	account, ok := st.Origin.(*protocol.DataAccount)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v",
			protocol.AccountTypeDataAccount, st.Origin.Type())
	}

	if scratch && !account.Scratch {
		return nil, fmt.Errorf("cannot write scratch data to a non-scratch account")
	}

	// now replace the transaction payload with a segregated witness to the data.
	// This technique is used to segregate the payload from the stored transaction
	// by replacing the data payload with a smaller reference to that data via the
	// entry hash.  While technically, not true segwit since the entry hash is not
	// signed, the original transaction can be reconstructed by recombining the
	// signature info stored on the pending chain, the transaction info stored on
	// the main chain, and the original data payload that resides on the data chain
	// when the user wishes to validate the signature of the transaction that
	// produced the data entry

	sw := protocol.SegWitDataEntry{}
	sw.Cause = cause
	sw.EntryHash = *(*[32]byte)(entry.Hash())
	sw.EntryUrl = st.OriginUrl

	//now replace the original data entry payload with the new segwit payload
	txn.Body = &sw

	//store the entry
	st.UpdateData(st.Origin, sw.EntryHash[:], entry)

	result := new(protocol.WriteDataResult)
	result.EntryHash = *(*[32]byte)(entry.Hash())
	result.AccountID = st.OriginUrl.AccountID()
	result.AccountUrl = st.OriginUrl
	return result, nil
}
