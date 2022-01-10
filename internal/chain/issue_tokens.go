package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type IssueTokens struct{}

func (IssueTokens) Type() types.TxType { return types.TxTypeIssueTokens }

func (IssueTokens) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.IssueTokens)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err := url.Parse(body.Recipient)
	if err != nil {
		return fmt.Errorf("invalid account URL: %v", err)
	}

	var tokenAccount *protocol.TokenIssuer
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.TokenIssuer:
			tokenAccount = origin
		default:
			return fmt.Errorf("invalid origin record: want chain type %v, got %v", types.ChainTypeTokenIssuer, origin.Header().Type)
		}
	}

	if tokenAccount.Supply.Cmp(&body.Amount) < 0 && tokenAccount.HasSupplyLimit {
		return fmt.Errorf("can't issue more than the limited supply")
	}
	tokenAccount.Supply.Sub(&tokenAccount.Supply, &body.Amount)

	account := protocol.NewLiteTokenAccount()
	account.ChainUrl = types.String(accountUrl.String())
	account.TokenUrl = tokenAccount.Header().GetChainUrl()

	if !account.CreditTokens(&body.Amount) {
		return fmt.Errorf("unable to add deposit balance to account")
	}
	st.Update(account, tokenAccount)

	return nil
}
