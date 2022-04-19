package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticForwardTransaction struct{}

func (SyntheticForwardTransaction) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticForwardTransaction
}

func (SyntheticForwardTransaction) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticForwardTransaction{}).Validate(st, tx)
}

func (SyntheticForwardTransaction) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticForwardTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticForwardTransaction), tx.Transaction.Body)
	}

	// Submit the transaction for processing
	d := tx.NewForwarded(body)
	st.state.ProcessAdditionalTransaction(d)
	return nil, nil
}
