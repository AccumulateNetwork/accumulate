package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticDepositCredits struct{}

func (SyntheticDepositCredits) Type() types.TxType { return types.TxTypeSyntheticDepositCredits }

func (SyntheticDepositCredits) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticDepositCredits)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	var account creditChain
	switch sponsor := st.Sponsor.(type) {
	case *protocol.AnonTokenAccount:
		account = sponsor

	case *protocol.KeyPage:
		account = sponsor

	default:
		return fmt.Errorf("invalid sponsor: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeKeyPage, st.Sponsor.Header().Type)
	}

	account.CreditCredits(body.Amount)
	st.Update(account)
	return nil
}
