package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type InternalTransactionsSent struct{}

func (InternalTransactionsSent) Type() types.TxType { return types.TxTypeInternalTransactionsSent }

func (InternalTransactionsSent) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.InternalTransactionsSent)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.InternalTransactionsSent), tx.Transaction.Body)
	}

	ledger, ok := st.Origin.(*protocol.InternalLedger)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeInternalLedger, st.Origin.GetType())
	}

	confirmed := map[[32]byte]bool{}
	for _, id := range body.Transactions {
		confirmed[id] = true
	}

	unsent := ledger.Synthetic.Unsent
	ledger.Synthetic.Unsent = make([][32]byte, 0, len(unsent))

	for _, id := range unsent {
		if !confirmed[id] {
			ledger.Synthetic.Unsent = append(ledger.Synthetic.Unsent, id)
			continue
		}
	}

	st.Update(ledger)
	return nil, nil
}
