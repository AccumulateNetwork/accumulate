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

	var account tokenChain
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteTokenAccount:
			account = origin
		case *protocol.TokenAccount:
			account = origin
		default:
			return fmt.Errorf("invalid origin record: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeTokenAccount, origin.Header().Type)
		}
	} else if keyHash, _, err := protocol.ParseLiteAddress(tx.Transaction.Origin); err != nil {
		return fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return fmt.Errorf("could not find token account")
	}

	if !account.DebitTokens(&body.Amount) {
		return fmt.Errorf("unable to debit balance from account")
	}
	st.Update(account)
	return nil
}
