package chain

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SyntheticReceipt struct{}

func (SyntheticReceipt) Type() protocol.TransactionType {
	return protocol.TransactionTypeSyntheticReceipt
}

func (SyntheticReceipt) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SyntheticMirror)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SyntheticMirror), tx.Transaction.Body)
	}

}
