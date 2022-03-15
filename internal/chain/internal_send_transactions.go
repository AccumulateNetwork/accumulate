package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type InternalSendTransactions struct{}

func (InternalSendTransactions) Type() types.TxType { return types.TxTypeInternalSendTransactions }

func (InternalSendTransactions) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.InternalSendTransactions)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.InternalSendTransactions), tx.Transaction.Body)
	}

	for _, tx := range body.Transactions {
		st.Submit(tx.Recipient, tx.Payload)
	}

	return nil, nil
}
