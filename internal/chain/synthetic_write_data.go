package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticWriteData struct{}

func (SyntheticWriteData) Type() types.TxType { return types.TxTypeSyntheticWriteData }

func (SyntheticWriteData) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticWriteData)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	return errors.New("not implemented") // TODO
}
