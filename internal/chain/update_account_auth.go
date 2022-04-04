package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateAccountAuth struct{}

func (UpdateAccountAuth) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateAccountAuth
}

func (UpdateAccountAuth) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (UpdateAccountAuth{}).Validate(st, tx)
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

		default:
			return nil, fmt.Errorf("invalid operation: %v", op.Type())
		}
	}

	st.Update(st.Origin)
	return nil, nil
}
