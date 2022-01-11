package chain

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type WriteDataTo struct{}

func (WriteDataTo) Type() types.TransactionType { return types.TxTypeWriteDataTo }

func (WriteDataTo) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.WriteDataTo)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	recipient, err := url.Parse(body.Recipient)
	if err != nil {
		return err
	}

	if _, err := protocol.ParseLiteDataAddress(recipient); err != nil {
		return fmt.Errorf("only writes to lite data accounts supported: %s: %v", recipient, err)
	}

	writeThis := new(protocol.SyntheticWriteData)
	writeThis.Entry = body.Entry
	copy(writeThis.Cause[:], tx.Transaction.Hash())

	st.Submit(recipient, writeThis)

	return nil
}
