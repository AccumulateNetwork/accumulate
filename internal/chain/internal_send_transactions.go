package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type InternalSendTransactions struct{}

func (InternalSendTransactions) Type() types.TxType { return types.TxTypeInternalSendTransactions }

func (InternalSendTransactions) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.InternalSendTransactions)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	for _, tx := range body.Transactions {
		u, err := url.Parse(tx.Recipient)
		if err != nil {
			return fmt.Errorf("invalid recipient: %v", err)
		}

		st.Submit(u, &tx.Payload)
	}

	return nil
}
