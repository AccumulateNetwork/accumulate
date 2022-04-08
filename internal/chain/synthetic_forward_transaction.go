package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticForwardTransaction struct{}

func (SyntheticForwardTransaction) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticForwardTransaction
}

func (SyntheticForwardTransaction) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (SyntheticForwardTransaction{}).Validate(st, tx)
}

func (SyntheticForwardTransaction) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticForwardTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticForwardTransaction), tx.Transaction.Body)
	}

	// Construct an envelope
	env := new(protocol.Envelope)
	env.Signatures = make([]protocol.Signature, len(body.Signatures))
	env.Transaction = body.Transaction
	if body.TransactionHash != nil {
		env.TxHash = body.TransactionHash
	}
	for i, sig := range body.Signatures {
		env.Signatures[i] = &sig
	}

	// Submit the envelope for processing
	st.state.ProcessAdditionalTransaction(env)
	return nil, nil
}
