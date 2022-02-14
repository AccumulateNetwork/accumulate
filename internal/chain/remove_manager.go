package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type RemoveManager struct{}

func (RemoveManager) Type() protocol.TransactionType { return protocol.TransactionTypeRemoveManager }

func (RemoveManager) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	_, ok := tx.Transaction.Body.(*protocol.RemoveManager)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.RemoveManager), tx.Transaction.Body)
	}
	if st.Origin.Header().ManagerKeyBook == "" {
		return nil, fmt.Errorf("manager keybook not assigned")
	}

	chain := st.Origin
	chain.Header().ManagerKeyBook = ""
	st.Update(chain)
	return nil, nil
}
