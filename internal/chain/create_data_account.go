package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateDataAccount struct{}

func (CreateDataAccount) Type() types.TxType { return types.TxTypeCreateDataAccount }

func (CreateDataAccount) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.CreateDataAccount)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	return errors.New("not implemented") // TODO
}
