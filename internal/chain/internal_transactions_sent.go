package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type InternalTransactionsSent struct{}

func (InternalTransactionsSent) Type() protocol.TransactionType {
	return protocol.TransactionTypeInternalTransactionsSent
}

func (InternalTransactionsSent) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (InternalTransactionsSent{}).Validate(st, tx)
}

func (InternalTransactionsSent) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.InternalTransactionsSent)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.InternalTransactionsSent), tx.Transaction.Body)
	}

	// Make a map of transactions that have been sent
	confirmed := map[[32]byte]bool{}
	for _, id := range body.Transactions {
		confirmed[id] = true
	}

	// Update the main ledger
	ledger, ok := st.Origin.(*protocol.InternalLedger)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeInternalLedger, st.Origin.Type())
	}

	unsent := ledger.Synthetic.Unsent
	ledger.Synthetic.Unsent = make([][32]byte, 0, len(unsent))
	for _, id := range unsent {
		if !confirmed[id] {
			ledger.Synthetic.Unsent = append(ledger.Synthetic.Unsent, id)
			continue
		}
		st.logger.Debug("Did send transaction", "txid", logging.AsHex(id).Slice(0, 4), "module", "governor")
	}

	st.Update(ledger)

	return nil, nil
}
