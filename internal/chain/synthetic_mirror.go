package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticMirror struct{}

func (SyntheticMirror) Type() protocol.TransactionType {
	return protocol.TransactionTypeMirrorSystemRecords
}

func (SyntheticMirror) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SyntheticMirror{}).Validate(st, tx)
}

func (SyntheticMirror) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.MirrorSystemRecords)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.MirrorSystemRecords), tx.Transaction.Body)
	}

	for _, obj := range body.Objects {
		// TODO Check merkle tree

		// TODO Save the merkle state somewhere?
		record := obj.Account
		st.logger.Debug("Mirroring", "url", record.GetUrl())
		err := st.Update(record)
		if err != nil {
			return nil, fmt.Errorf("failed to update %v: %v", record.GetUrl(), err)
		}
	}

	return nil, nil
}
