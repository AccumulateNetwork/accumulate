package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type InternalTransactionsSigned struct{}

func (InternalTransactionsSigned) Type() protocol.TransactionType {
	return protocol.TransactionTypeInternalTransactionsSigned
}

func (InternalTransactionsSigned) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (InternalTransactionsSigned{}).Validate(st, tx)
}

func (InternalTransactionsSigned) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.InternalTransactionsSigned)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.InternalTransactionsSigned), tx.Transaction.Body)
	}

	ledger, ok := st.Origin.(*protocol.InternalLedger)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeInternalLedger, st.Origin.Type())
	}

	signatures := map[[32]byte]protocol.Signature{}
	for _, tx := range body.Transactions {
		signatures[tx.Transaction] = tx.Signature
	}

	unsigned := ledger.Synthetic.Unsigned
	ledger.Synthetic.Unsigned = make([][32]byte, 0, len(unsigned))

	for _, id := range unsigned {
		// Make a new variable to avoid the evil that is taking a pointer to a
		// loop variable
		id := id

		sig := signatures[id]
		if sig == nil {
			ledger.Synthetic.Unsigned = append(ledger.Synthetic.Unsigned, id)
			continue
		}

		// Load the transaction
		txn, err := st.LoadTxn(id)
		if err != nil {
			return nil, err
		}

		// Add the signature
		env := new(protocol.Envelope)
		env.Transaction = txn
		env.Signatures = []protocol.Signature{sig}

		// Validate it
		if !env.Verify() {
			return nil, fmt.Errorf("invalid signature for txn %X", id)
		}

		// Write the signature
		st.SignTransaction(id[:], sig)

		// Send the transaction
		ledger.Synthetic.Unsent = append(ledger.Synthetic.Unsent, id)
		st.logger.Debug("Did sign transaction",
			"type", txn.Body.Type(),
			"txid", logging.AsHex(id).Slice(0, 4),
			"module", "governor")
	}

	st.Update(ledger)
	return nil, nil
}
