package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type RemoveManager struct{}

func (RemoveManager) Type() types.TxType { return types.TxTypeRemoveManager }

func (RemoveManager) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.RemoveManager)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}
	if st.Origin.Header().ManagerKeyBook == "" {
		return nil, fmt.Errorf("manager keybook not assigned")
	}

	chain := st.Origin
	chain.Header().ManagerKeyBook = ""
	st.Update(chain)
	return nil, nil
}
