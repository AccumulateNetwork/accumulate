package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateAccountAuth struct{}

func (UpdateAccountAuth) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateAccountAuth
}

func (UpdateAccountAuth) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateAccountAuth)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateAccountAuth), tx.Transaction.Body)
	}

	for _, op := range body.Operations {
		switch op := op.(type) {
		case *protocol.EnableAccountAuthOperation:
			if !st.Origin.Header().KeyBook.Equal(op.Authority) {
				return nil, fmt.Errorf("cannot enable auth for %v", op.Authority)
			}
			st.Origin.Header().AuthDisabled = false

		case *protocol.DisableAccountAuthOperation:
			if !st.Origin.Header().KeyBook.Equal(op.Authority) {
				return nil, fmt.Errorf("cannot enable auth for %v", op.Authority)
			}
			st.Origin.Header().AuthDisabled = true

		case *protocol.AddAccountAuthorityOperation:
			if st.Origin.Header().ManagerKeyBook != nil {
				return nil, fmt.Errorf("manager keybook already assigned")
			}

			chain := st.Origin
			chain.Header().ManagerKeyBook = op.Authority
			st.Update(chain)

		case *protocol.RemoveAccountAuthorityOperation:
			if st.Origin.Header().ManagerKeyBook == nil {
				return nil, fmt.Errorf("manager keybook not assigned")
			}

			chain := st.Origin
			chain.Header().ManagerKeyBook = nil
			st.Update(chain)

		default:
			return nil, fmt.Errorf("invalid operation: %v", op.Type())
		}
	}

	st.Update(st.Origin)
	return nil, nil
}
