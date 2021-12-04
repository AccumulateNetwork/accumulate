package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type WriteData struct{}

func (WriteData) Type() types.TxType { return types.TxTypeWriteData }

func (WriteData) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.WriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	//	entryHash := protocol.ComputeEntryHash(append(body.ExtIds, body.Data))

	return errors.New("not implemented") // TODO
}
