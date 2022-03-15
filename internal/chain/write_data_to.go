package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type WriteDataTo struct{}

func (WriteDataTo) Type() types.TransactionType { return types.TxTypeWriteDataTo }

func (WriteDataTo) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.WriteDataTo)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.WriteDataTo), tx.Transaction.Body)
	}

	if _, err := protocol.ParseLiteDataAddress(body.Recipient); err != nil {
		return nil, fmt.Errorf("only writes to lite data accounts supported: %s: %v", body.Recipient, err)
	}

	writeThis := new(protocol.SyntheticWriteData)
	writeThis.Cause = *(*[32]byte)(tx.GetTxHash())
	writeThis.Entry = body.Entry

	st.Submit(body.Recipient, writeThis)

	return nil, nil
}
