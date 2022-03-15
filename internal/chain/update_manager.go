package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateManager struct{}

func (UpdateManager) Type() protocol.TransactionType { return protocol.TransactionTypeUpdateManager }

func (UpdateManager) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateManager)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateManager), tx.Transaction.Body)
	}
	if st.Origin.Header().ManagerKeyBook != nil {
		return nil, fmt.Errorf("manager keybook already assigned")
	}

	chain := st.Origin
	chain.Header().ManagerKeyBook = body.ManagerKeyBook
	st.Update(chain)
	return nil, nil
}
