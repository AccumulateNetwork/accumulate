package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type InternalSendTransactions struct{}

func (InternalSendTransactions) Type() types.TxType { return types.TxTypeInternalSendTransactions }

func (InternalSendTransactions) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.InternalSendTransactions)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	for _, tx := range body.Transactions {
		st.Submit(tx.Recipient, tx.Payload)
	}

	return nil, nil
}
