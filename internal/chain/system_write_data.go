package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SystemWriteData struct{}

func (SystemWriteData) Type() protocol.TransactionType {
	return protocol.TransactionTypeSystemWriteData
}

func (SystemWriteData) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SystemWriteData{}).Validate(st, tx)
}

func (SystemWriteData) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SystemWriteData)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SystemWriteData), tx.Transaction.Body)
	}

	if partition, ok := protocol.ParsePartitionUrl(st.OriginUrl); !ok {
		return nil, errors.Format(errors.StatusBadRequest, "invalid principal: %v is not a system account", st.OriginUrl)
	} else if partition != st.PartitionId {
		return nil, errors.Format(errors.StatusBadRequest, "invalid principal: %v belongs to the wrong partition", st.OriginUrl)
	}

	return executeWriteFullDataAccount(st, body.Entry, false, body.WriteToState)
}
