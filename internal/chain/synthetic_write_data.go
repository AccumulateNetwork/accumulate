package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types/state"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticWriteData struct{}

func (SyntheticWriteData) Type() types.TransactionType { return types.TxTypeSyntheticWriteData }

func (SyntheticWriteData) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.SyntheticWriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	var account state.Chain

	sw := protocol.SegWitDataEntry{}
	//we provide the transaction id of the original transaction
	copy(sw.Cause[:], body.Cause[:])

	if st.Origin != nil {
		//reference this chain in the segwit entry.
		sw.EntryUrl = st.Origin.Header().GetChainUrl()

		switch origin := st.Origin.(type) {
		case *protocol.LiteDataAccount:
			liteDataAccountId, err := protocol.ParseLiteDataAddress(tx.Transaction.Origin)
			if err != nil {
				return err
			}
			//reconstruct full lite chain id
			liteDataAccountId = append(liteDataAccountId, origin.Tail...)

			//compute the hash for this entry
			entryHash, err := protocol.ComputeLiteEntryHashFromEntry(liteDataAccountId, &body.Entry)
			if err != nil {
				return err
			}
			copy(sw.EntryHash[:], entryHash)
			account = origin
		case *protocol.DataAccount:
			//synthetic data writes to adi data accounts are not yet permitted.
			copy(sw.EntryHash[:], body.Entry.Hash())
			account = origin
			panic("synthetic writes to adi data accounts are not currently supported")
		default:
			return fmt.Errorf("invalid origin record: want chain type %v or %v, got %v",
				types.ChainTypeLiteDataAccount, types.ChainTypeDataAccount, origin.Header().Type)
		}
	} else if _, err := protocol.ParseLiteDataAddress(tx.Transaction.Origin); err != nil {
		return fmt.Errorf("invalid lite data URL %s: %v", tx.Transaction.Origin.String(), err)
	} else {
		// Address is lite, but the lite data account doesn't exist, so create one
		liteDataAccountId := protocol.ComputeLiteDataAccountId(&body.Entry)
		u, err := protocol.LiteDataAddress(liteDataAccountId)
		if err != nil {
			return err
		}
		originUrl := tx.Transaction.Origin.String()

		//the computed data account chain id must match the origin url.
		if u.String() != originUrl {
			return fmt.Errorf("first entry doesnt match chain id")
		}

		lite := protocol.NewLiteDataAccount()
		lite.ChainUrl = types.String(originUrl)
		//we store the tail of the lite data account id in the state. The first part
		//of the lite data account can be obtained from the LiteDataAddress. When
		//we want to reference the chain, we have all the info we need at the cost
		//of 12 bytes.  The advantage is we avoid a lookup of entry 0, and
		//computation of the chain id. The disadvantage is we have to store
		//12 additional bytes.
		lite.Tail = liteDataAccountId[20:32]

		entryHash, err := protocol.ComputeLiteEntryHashFromEntry(liteDataAccountId, &body.Entry)
		if err != nil {
			return err
		}
		copy(sw.EntryHash[:], entryHash)
		sw.EntryUrl = originUrl

		account = lite
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

	//now replace the original data entry payload with the new segwit payload
	segWitPayload, err := sw.MarshalBinary()
	if err != nil {
		return fmt.Errorf("unable to marshal segwit, %v", err)
	}
	tx.Transaction.Body = segWitPayload

	st.UpdateData(account, sw.EntryHash[:], &body.Entry)

	return nil
}
