package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticBurnTokens struct{}

func (SyntheticBurnTokens) Type() types.TxType { return types.TxTypeSyntheticBurnTokens }

func (SyntheticBurnTokens) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticBurnTokens)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	return errors.New("not implemented") // TODO
}
