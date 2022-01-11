package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type InternalTransactionsSent struct{}

func (InternalTransactionsSent) Type() types.TxType { return types.TxTypeInternalTransactionsSent }

func (InternalTransactionsSent) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.InternalTransactionsSent)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	ledger, ok := st.Origin.(*protocol.InternalLedger)
	if !ok {
		return fmt.Errorf("invalid origin record: want account type %v, got %v", types.AccountTypeInternalLedger, st.Origin.Header().Type)
	}

	sent := map[[32]byte]bool{}
	for _, id := range body.Transactions {
		sent[id] = true
	}

	unsent := ledger.Synthetic.Unsent
	ledger.Synthetic.Unsent = make([][32]byte, 0, len(unsent))

	for _, id := range unsent {
		if !sent[id] {
			ledger.Synthetic.Unsent = append(ledger.Synthetic.Unsent, id)
			continue
		}
	}

	st.Update(ledger)
	return nil
}
