package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type RemoveManager struct{}

func (RemoveManager) Type() protocol.TransactionType { return protocol.TransactionTypeRemoveManager }

func (RemoveManager) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	_, ok := tx.Transaction.Body.(*protocol.RemoveManager)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.RemoveManager), tx.Transaction.Body)
	}
	if st.Origin.Header().ManagerKeyBook == nil {
		return nil, fmt.Errorf("manager keybook not assigned")
	}

	chain := st.Origin
	chain.Header().ManagerKeyBook = nil
	st.Update(chain)
	return nil, nil
}
