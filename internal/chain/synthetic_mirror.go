package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticMirror struct{}

func (SyntheticMirror) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticMirror
}

func (SyntheticMirror) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticMirror)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticMirror), tx.Transaction.Body)
	}

	for _, obj := range body.Objects {
		// TODO Check merkle tree

		// Unmarshal the record
		record, err := protocol.UnmarshalAccount(obj.Record)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal record: %v", err)
		}

		// TODO Save the merkle state somewhere?
		st.logger.Debug("Mirroring", "url", record.GetUrl())
		st.Update(record)
	}

	return nil, nil
}
