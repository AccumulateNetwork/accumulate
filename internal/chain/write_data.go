package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type WriteData struct{}

func (WriteData) Type() types.TransactionType { return types.TxTypeWriteData }

func (WriteData) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.WriteData)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	if st.Origin.Header().Type != types.AccountTypeDataAccount {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v",
			types.AccountTypeDataAccount, st.Origin.Header().Type)
	}

	//check will return error if there is too much data or no data for the entry
	_, err = body.Entry.CheckSize()
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
	sw.EntryUrl = tx.Transaction.Origin.String()

	segWitPayload, err := sw.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to marshal segwit, %v", err)
	}

	//now replace the original data entry payload with the new segwit payload
	tx.Transaction.Body = segWitPayload

	//store the entry
	st.UpdateData(st.Origin, sw.EntryHash[:], &body.Entry)

	result := new(protocol.WriteDataResult)
	result.EntryHash = *(*[32]byte)(body.Entry.Hash())
	result.AccountID = tx.Transaction.Origin.AccountID()
	result.AccountUrl = tx.Transaction.Origin
	return result, nil
}
