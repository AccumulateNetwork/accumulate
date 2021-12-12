package chain

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticDepositCredits struct{}

func (SyntheticDepositCredits) Type() types.TransactionType {
	return types.TxTypeSyntheticDepositCredits
}

func (SyntheticDepositCredits) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.SyntheticDepositCredits)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return fmt.Errorf("invalid recipient URL: %v", err)
	}

	var account creditChain
	if st.Sponsor != nil {
		switch sponsor := st.Sponsor.(type) {
		case *protocol.LiteTokenAccount:
			account = sponsor
		case *protocol.KeyPage:
			account = sponsor
		default:
			return fmt.Errorf("invalid sponsor: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeKeyPage, st.Sponsor.Header().Type)
		}
	} else if keyHash, tok, err := protocol.ParseLiteAddress(accountUrl); err != nil {
		return fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return fmt.Errorf("could not find token account")
	} else {
		// Address is lite and the account doesn't exist, so create one
		lite := protocol.NewLiteTokenAccount()
		lite.ChainUrl = types.String(accountUrl.String())
		lite.TokenUrl = tok.String()
		account = lite
	}
	account.CreditCredits(body.Amount)
	st.Update(account)
	return nil
}
