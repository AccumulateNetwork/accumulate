package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type SyntheticWriteData struct{}

func (SyntheticWriteData) Type() types.TxType { return types.TxTypeSyntheticWriteData }

func (SyntheticWriteData) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticWriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	sw := protocol.SegWitDataEntry{}
	var chainId []byte
	chainUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return fmt.Errorf("invalid recipient URL: %v", err)
	}

	var dataPayload []byte
	var account state.Chain

	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteDataAccount:
			account = origin
		case *protocol.DataAccount:
			account = origin
			copy(sw.EntryHash[:], body.Entry.Hash())
			dataPayload, err = body.Entry.MarshalBinary()
			if err != nil {
				return fmt.Errorf("error marshaling data entry, %v", err)
			}
		default:
			return fmt.Errorf("invalid origin record: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeTokenAccount, origin.Header().Type)
		}
	} else if _, err := protocol.ParseLiteChainAddress(chainUrl); err != nil {
		return fmt.Errorf("invalid lite chain URL: %v", err)
	} else if err != nil {
		return err
	} else {
		// Address is lite and the chain doesn't exist, so create one
		liteChainId := protocol.ComputeLiteChainId(&body.Entry)
		u, err := protocol.LiteDataAddress(chainId)
		if err != nil {
			return err
		}

		if u.String() != chainUrl.String() {
			return fmt.Errorf("first entry doesnt match chain id")
		}

		lite := protocol.NewLiteDataAccount()
		lite.ChainUrl = types.String(chainUrl.String())
		//we store the tail of the chain Id in the state.  the first part
		//of the chainid can be obtained from the ChainAddress.
		lite.Tail = liteChainId[20:32]

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

	//dataPayload will be nil for a lite data account, so need to populate it
	if dataPayload == nil {

		liteChainId, err := protocol.ParseLiteChainAddress(chainUrl)
		if err != nil {
			return err
		}

		origin := st.Origin.(*protocol.LiteDataAccount)
		//reconstruct full lite chain id
		liteChainId = append(liteChainId, origin.Tail...)

		lde := protocol.LiteDataEntry{}
		copy(lde.ChainId[:], liteChainId[:])
		lde.DataEntry = body.Entry

		dataPayload, err = lde.MarshalBinary()
		if err != nil {
			return fmt.Errorf("error marshaling lite data entry, %v", err)
		}

		entryHash, err := lde.Hash()
		if err != nil {
			return err
		}
		copy(sw.EntryHash[:], entryHash)

	}
	copy(sw.Cause[:], tx.TransactionHash())
	sw.EntryUrl = account.Header().GetChainUrl()

	segWitPayload, err := sw.MarshalBinary()
	if err != nil {
		return fmt.Errorf("unable to marshal segwit, %v", err)
	}

	//now replace the original data entry payload with the new segwit payload
	tx.Transaction = segWitPayload

	st.UpdateData(st.Origin, sw.EntryHash[:], dataPayload)

	return nil
}
