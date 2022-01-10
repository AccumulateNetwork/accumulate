package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type SyntheticWriteData struct{}

func (SyntheticWriteData) Type() types.TransactionType { return types.TxTypeSyntheticWriteData }

func (SyntheticWriteData) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.SyntheticWriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	sw := protocol.SegWitDataEntry{}

	var account state.Chain
	haveLiteChain := true

	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteDataAccount:
			account = origin
		case *protocol.DataAccount:
			account = origin
			copy(sw.EntryHash[:], body.Entry.Hash())
			haveLiteChain = false
		default:
			return fmt.Errorf("invalid origin record: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeTokenAccount, origin.Header().Type)
		}
	} else if _, err := protocol.ParseLiteChainAddress(tx.Transaction.Origin); err != nil {
		return fmt.Errorf("invalid lite chain URL: %v", err)
	} else if err != nil {
		return err
	} else {
		// Address is lite and the chain doesn't exist, so create one
		liteDataChainId := protocol.ComputeLiteDataAccountId(&body.Entry)
		u, err := protocol.LiteDataAddress(liteDataChainId)
		if err != nil {
			return err
		}

		//the computed data account chain id must match the origin url.
		if u.String() != tx.Transaction.Origin.String() {
			return fmt.Errorf("first entry doesnt match chain id")
		}

		lite := protocol.NewLiteDataAccount()
		lite.ChainUrl = types.String(tx.Transaction.Origin.String())
		//we store the tail of the lite data account id in the state. The first part
		//of the lite data account can be obtained from the ChainAddress. When
		//we want to reference the chain, we have all the info we need at the cost
		//of 12 bytes.  The advantage is we avoid a lookup of entry 0, and
		//computation of the chain id. The disadvantage is we have to store
		//12 additional bytes.
		lite.Tail = liteDataChainId[20:32]

		account = lite
	}

	if haveLiteChain {
		liteChainId, err := protocol.ParseLiteChainAddress(tx.Transaction.Origin)
		if err != nil {
			return err
		}

		origin := st.Origin.(*protocol.LiteDataAccount)

		//reconstruct full lite chain id
		liteChainId = append(liteChainId, origin.Tail...)

		lde := protocol.LiteDataEntry{}
		copy(lde.ChainId[:], liteChainId[:])
		lde.DataEntry = body.Entry

		entryHash, err := lde.Hash()
		if err != nil {
			return err
		}
		copy(sw.EntryHash[:], entryHash)
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

	//we provide the transaction id of the original transaction which caused this
	//synth write transaction
	copy(sw.Cause[:], body.Cause[:])

	sw.EntryUrl = account.Header().GetChainUrl()

	segWitPayload, err := sw.MarshalBinary()
	if err != nil {
		return fmt.Errorf("unable to marshal segwit, %v", err)
	}

	//now replace the original data entry payload with the new segwit payload
	tx.Transaction.Body = segWitPayload

	st.UpdateData(st.Origin, sw.EntryHash[:], &body.Entry)

	return nil
}
