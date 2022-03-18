package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type WriteDataTo struct{}

func (WriteDataTo) Type() protocol.TransactionType { return protocol.TransactionTypeWriteDataTo }

func (WriteDataTo) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteDataTo)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.WriteDataTo), tx.Transaction.Body)
	}

	if _, err := protocol.ParseLiteDataAddress(body.Recipient); err != nil {
		return nil, fmt.Errorf("only writes to lite data accounts supported: %s: %v", body.Recipient, err)
	}

	writeThis := new(protocol.SyntheticWriteData)
	writeThis.SetSyntheticOrigin(tx.GetTxHash(), st.OriginUrl)
	writeThis.Entry = body.Entry

	st.Submit(body.Recipient, writeThis)

	return nil, nil
}
