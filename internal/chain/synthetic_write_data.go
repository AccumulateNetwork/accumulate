package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticWriteData struct{}

func (SyntheticWriteData) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticWriteData
}

func (SyntheticWriteData) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticWriteData{}).Validate(st, tx)
}

func (SyntheticWriteData) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticWriteData)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticWriteData), tx.Transaction.Body)
	}

	return executeWriteLiteDataAccount(st, body.Entry, false)
}

func executeWriteLiteDataAccount(st *StateManager, entry protocol.DataEntry, scratch bool) (protocol.TransactionResult, error) {
	if scratch {
		return nil, fmt.Errorf("cannot write scratch data to a lite data account")
	}

	var account protocol.Account
	result := new(protocol.WriteDataResult)
	if st.Origin != nil {

		switch origin := st.Origin.(type) {
		case *protocol.LiteDataAccount:
			liteDataAccountId, err := protocol.ParseLiteDataAddress(st.OriginUrl)
			if err != nil {
				return nil, err
			}

			//compute the hash for this entry
			entryHash, err := protocol.ComputeFactomEntryHashForAccount(liteDataAccountId, entry.GetData())
			if err != nil {
				return nil, err
			}

			account = origin
			result.EntryHash = *(*[32]byte)(entryHash)
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
			return nil, fmt.Errorf("first entry doesnt match chain id")
		}

		lite := new(protocol.LiteDataAccount)
		lite.Url = u

		entryHash, err := protocol.ComputeFactomEntryHashForAccount(liteDataAccountId, entry.GetData())
		if err != nil {
			return nil, err
		}

		account = lite
		result.EntryHash = *(*[32]byte)(entryHash)
		result.AccountID = liteDataAccountId
		result.AccountUrl = u
	}

	st.UpdateData(account, result.EntryHash[:], entry)

	return result, nil
}
