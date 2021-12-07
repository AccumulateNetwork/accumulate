package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type WriteData struct{}

func (WriteData) Type() types.TransactionType { return types.TxTypeWriteData }

func (WriteData) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.WriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if st.Sponsor.Header().Type != types.ChainTypeDataAccount {
		return fmt.Errorf("sponsor is not a data account: want %v, got %v",
			types.ChainTypeDataAccount, st.Sponsor.Header().Type)
	}

	//check will return error if there is too much data or no data for the entry
	_, err = body.Entry.CheckSize()
	if err != nil {
		return err
	}

	//now replace the transaction payload with a segregated witness to the data
	sw := protocol.SegWitDataEntry{}
	copy(sw.EntryHash[:], body.Entry.Hash())
	sw.EntryUrl = st.Sponsor.Header().GetChainUrl()

	segWitPayload, err := sw.MarshalBinary()
	if err != nil {
		return fmt.Errorf("unable to marshal segwit, %v", err)
	}

	dataPayload := tx.Transaction
	tx.Transaction = segWitPayload

	dataAccountWithCache := protocol.NewDataAccountStateCache(
		st.Sponsor.(*protocol.DataAccount), sw.EntryHash[:], dataPayload)

	st.UpdateData(dataAccountWithCache)

	return nil
}
