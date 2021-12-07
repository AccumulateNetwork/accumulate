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

	//cost will return error if there is too much data or no data for the entry
	_, err = body.Entry.CheckSize()
	if err != nil {
		return err
	}

	//todo: need to deduct credits from page, should this be done by check?
	_, _ = body.Entry.Cost()
	//? st.DeductFee(page, cost)

	return nil
}
