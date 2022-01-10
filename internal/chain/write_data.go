package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type WriteData struct{}

func (WriteData) Type() types.TransactionType { return types.TxTypeWriteData }

func (WriteData) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.WriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if st.Origin.Header().Type != types.ChainTypeDataAccount {
		return fmt.Errorf("invalid origin record: want %v, got %v",
			types.ChainTypeDataAccount, st.Origin.Header().Type)
	}

	//check will return error if there is too much data or no data for the entry
	_, err = body.Entry.CheckSize()
	if err != nil {
		return err
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
	copy(sw.Cause[:], tx.Transaction.Hash())
	copy(sw.EntryHash[:], body.Entry.Hash())
	sw.EntryUrl = st.Origin.Header().GetChainUrl()

	segWitPayload, err := sw.MarshalBinary()
	if err != nil {
		return fmt.Errorf("unable to marshal segwit, %v", err)
	}

	//now replace the original data entry payload with the new segwit payload
	tx.Transaction.Body = segWitPayload

	//store the entry
	st.UpdateData(st.Origin, sw.EntryHash[:], &body.Entry)

	return nil
}
