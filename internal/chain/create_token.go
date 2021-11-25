package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateToken struct{}

func (CreateToken) Type() types.TxType { return types.TxTypeCreateToken }

func (CreateToken) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.CreateToken)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	return errors.New("not implemented") // TODO
}
