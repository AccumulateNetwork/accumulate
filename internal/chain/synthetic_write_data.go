package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticWriteData struct{}

func (SyntheticWriteData) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticWriteData
}

func (SyntheticWriteData) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (SyntheticWriteData{}).Validate(st, tx)
}

func (SyntheticWriteData) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticWriteData)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticWriteData), tx.Transaction.Body)
	}

	var account protocol.Account
	result := new(protocol.WriteDataResult)
	if st.Origin != nil {

		switch origin := st.Origin.(type) {
		case *protocol.LiteDataAccount:
			liteDataAccountId, err := protocol.ParseLiteDataAddress(tx.Transaction.Header.Principal)
			if err != nil {
				return nil, err
			}
			//reconstruct full lite chain id
			liteDataAccountId = append(liteDataAccountId, origin.Tail...)

			//compute the hash for this entry
			entryHash, err := protocol.ComputeLiteEntryHashFromEntry(liteDataAccountId, &body.Entry)
			if err != nil {
				return nil, err
			}

			account = origin
			result.EntryHash = *(*[32]byte)(entryHash)
			result.AccountID = liteDataAccountId
			result.AccountUrl = tx.Transaction.Header.Principal
		default:
			return nil, fmt.Errorf("invalid origin record: want chain type %v, got %v",
				protocol.AccountTypeLiteDataAccount, origin.GetType())
		}
	} else if _, err := protocol.ParseLiteDataAddress(tx.Transaction.Header.Principal); err != nil {
		return nil, fmt.Errorf("invalid lite data URL %s: %v", tx.Transaction.Header.Principal.String(), err)
	} else {
		// Address is lite, but the lite data account doesn't exist, so create one
		liteDataAccountId := protocol.ComputeLiteDataAccountId(&body.Entry)
		u, err := protocol.LiteDataAddress(liteDataAccountId)
		if err != nil {
			return nil, err
		}

		//the computed data account chain id must match the origin url.
		if !tx.Transaction.Header.Principal.Equal(u) {
			return nil, fmt.Errorf("first entry doesnt match chain id")
		}

		lite := protocol.NewLiteDataAccount()
		lite.Url = u
		//we store the tail of the lite data account id in the state. The first part
		//of the lite data account can be obtained from the LiteDataAddress. When
		//we want to reference the chain, we have all the info we need at the cost
		//of 12 bytes.  The advantage is we avoid a lookup of entry 0, and
		//computation of the chain id. The disadvantage is we have to store
		//12 additional bytes.
		lite.Tail = liteDataAccountId[20:32]

		entryHash, err := protocol.ComputeLiteEntryHashFromEntry(liteDataAccountId, &body.Entry)
		if err != nil {
			return nil, err
		}

		account = lite
		result.EntryHash = *(*[32]byte)(entryHash)
		result.AccountID = liteDataAccountId
		result.AccountUrl = u
	}

	sw := protocol.SegWitDataEntry{}

	//reference this chain in the segwit entry.
	sw.EntryUrl = result.AccountUrl

	//we provide the transaction id of the original transaction
	sw.Cause = body.Cause

	//and the entry hash
	sw.EntryHash = result.EntryHash

	// now replace the transaction payload with a segregated witness to the data.
	// This technique is used to segregate the payload from the stored transaction
	// by replacing the data payload with a smaller reference to that data via the
	// entry hash.  While technically, not true segwit since the entry hash is not
	// signed, the original transaction can be reconstructed by recombining the
	// signature info stored on the pending chain, the transaction info stored on
	// the main chain, and the original data payload that resides on the data chain
	// when the user wishes to validate the signature of the transaction that
	// produced the data entry

	//now replace the original data entry payload with the new segwit payload
	tx.Transaction.Body = &sw

	st.UpdateData(account, sw.EntryHash[:], &body.Entry)

	return result, nil
}
