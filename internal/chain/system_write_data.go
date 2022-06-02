package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
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

	if subnet, ok := protocol.ParseSubnetUrl(st.OriginUrl); !ok {
		return nil, errors.Format(errors.StatusBadRequest, "invalid principal: %v is not a system account", st.OriginUrl)
	} else if subnet != st.SubnetId {
		return nil, errors.Format(errors.StatusBadRequest, "invalid principal: %v belongs to the wrong subnet", st.OriginUrl)
	}

	return executeWriteFullDataAccount(st, body.Entry, false, body.WriteToState)
}
