package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type BurnTokens struct{}

func (BurnTokens) Type() types.TxType { return types.TxTypeBurnTokens }

func (BurnTokens) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.BurnTokens)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	return errors.New("not implemented") // TODO
}
