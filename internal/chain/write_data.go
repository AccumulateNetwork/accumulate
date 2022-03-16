package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteData struct{}

func (WriteData) Type() protocol.TransactionType { return protocol.TransactionTypeWriteData }

func (WriteData) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteData)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.WriteData), tx.Transaction.Body)
	}

	if st.Origin.GetType() != protocol.AccountTypeDataAccount {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v",
			protocol.AccountTypeDataAccount, st.Origin.GetType())
	}

	//check will return error if there is too much data or no data for the entry
	_, err := body.Entry.CheckSize()
	if err != nil {
		return nil, err
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
	sw.Cause = *(*[32]byte)(tx.GetTxHash())
	sw.EntryHash = *(*[32]byte)(body.Entry.Hash())
	sw.EntryUrl = tx.Transaction.Origin

	//now replace the original data entry payload with the new segwit payload
	tx.Transaction.Body = &sw

	//store the entry
	st.UpdateData(st.Origin, sw.EntryHash[:], &body.Entry)

	result := new(protocol.WriteDataResult)
	result.EntryHash = *(*[32]byte)(body.Entry.Hash())
	result.AccountID = tx.Transaction.Origin.AccountID()
	result.AccountUrl = tx.Transaction.Origin
	return result, nil
}
