package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type WriteDataTo struct{}

func (WriteDataTo) Type() types.TransactionType { return types.TxTypeWriteDataTo }

func (WriteDataTo) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.WriteDataTo)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	recipient, err := url.Parse(body.Recipient)
	if err != nil {
		return nil, err
	}

	if _, err := protocol.ParseLiteDataAddress(recipient); err != nil {
		return nil, fmt.Errorf("only writes to lite data accounts supported: %s: %v", recipient, err)
	}

	writeThis := new(protocol.SyntheticWriteData)
	writeThis.Cause = *(*[32]byte)(tx.GetTxHash())
	writeThis.Entry = body.Entry

	st.Submit(recipient, writeThis)

	return nil, nil
}
