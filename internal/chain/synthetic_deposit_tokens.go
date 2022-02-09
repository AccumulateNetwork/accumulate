package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SyntheticDepositTokens struct{}

func (SyntheticDepositTokens) Type() types.TxType {
	return types.TxTypeSyntheticDepositTokens
}

func (SyntheticDepositTokens) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	// *big.Int, tokenChain, *url.URL
	body := new(protocol.SyntheticDepositTokens)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	tokenUrl, err := url.Parse(body.Token)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}

	var account tokenChain
	if st.Origin != nil {
		switch origin := st.Origin.(type) {
		case *protocol.LiteTokenAccount:
			account = origin
		case *protocol.TokenAccount:
			account = origin
		default:
			return nil, fmt.Errorf("invalid origin record: want account type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.Header().Type)
		}
	} else if keyHash, tok, err := protocol.ParseLiteTokenAddress(tx.Transaction.Origin); err != nil {
		return nil, fmt.Errorf("invalid lite token account URL: %v", err)
	} else if keyHash == nil {
		return nil, fmt.Errorf("could not find token account")
	} else if !tokenUrl.Equal(tok) {
		return nil, fmt.Errorf("token URL does not match lite token account URL")
	} else {
		// Address is lite and the account doesn't exist, so create one
		lite := protocol.NewLiteTokenAccount()
		lite.Url = tx.Transaction.Origin.String()
		lite.TokenUrl = tokenUrl.String()
		account = lite
	}

	if !account.CreditTokens(&body.Amount) {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}
	st.Update(account)

	return nil, nil
}
