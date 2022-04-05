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

func (InternalTransactionsSent) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
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

	// Update the synth ledger
	var synthLedger *protocol.InternalSyntheticLedger
	err := st.LoadUrlAs(st.nodeUrl.JoinPath(protocol.SyntheticLedgerPath), &synthLedger)
	if err != nil {
		return nil, fmt.Errorf("unable to load synthetic transaction ledger: %w", err)
	}

	pending := synthLedger.Pending
	synthLedger.Pending = make([]*protocol.SyntheticLedgerEntry, 0, len(pending))
	for _, entry := range pending {
		if !confirmed[entry.TransactionHash] {
			synthLedger.Pending = append(synthLedger.Pending, entry)
			continue
		}
	}

	st.Update(synthLedger)

	return nil, nil
}
