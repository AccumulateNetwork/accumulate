package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticBurnTokens struct{}

func (SyntheticBurnTokens) Type() types.TxType { return types.TxTypeSyntheticBurnTokens }

func (SyntheticBurnTokens) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.SyntheticBurnTokens)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	account := protocol.NewTokenIssuer()
	switch origin := st.Origin.(type) {
	case *protocol.TokenIssuer:
		account = origin
	default:
		return fmt.Errorf("invalid origin record: want chain type %v, got %v", types.AccountTypeTokenIssuer, origin.Header().Type)
	}

	account.Supply.Add(&account.Supply, &body.Amount)

	st.Update(account)
	return nil
}
