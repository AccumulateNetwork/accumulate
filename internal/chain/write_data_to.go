package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteDataTo struct{}

func (WriteDataTo) Type() protocol.TransactionType { return protocol.TransactionTypeWriteDataTo }

func (WriteDataTo) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (WriteDataTo{}).Validate(st, tx)
}

func (WriteDataTo) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteDataTo)
	if !ok {
		return nil, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.WriteDataTo), tx.Transaction.Body)
	}

	if body.Recipient == nil {
		return nil, errors.Format(errors.StatusBadRequest, "recipient is missing")
	}

	if body.Entry == nil {
		return nil, errors.Format(errors.StatusBadRequest, "entry is missing")
	}

	if _, err := protocol.ParseLiteDataAddress(body.Recipient); err != nil {
		return nil, errors.Format(errors.StatusBadRequest, "only writes to lite data accounts supported: %s: %v", body.Recipient, err)
	}

	writeThis := new(protocol.SyntheticWriteData)
	writeThis.Entry = body.Entry

	st.Submit(body.Recipient, writeThis)

	return nil, nil
}
