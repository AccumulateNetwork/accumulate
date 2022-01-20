package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateTokenAccount struct{}

func (CreateTokenAccount) Type() types.TxType { return types.TxTypeCreateTokenAccount }

func (CreateTokenAccount) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.CreateTokenAccount)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %v", err)
	}

	tokenUrl, err := url.Parse(body.TokenUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}
	// TODO Make sure tokenUrl is a real kind of token

	if !accountUrl.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q cannot be the origininator of %q", st.OriginUrl, accountUrl)
	}

	account := protocol.NewTokenAccount()
	account.ChainUrl = types.String(accountUrl.String())
	account.TokenUrl = tokenUrl.String()
	if body.KeyBookUrl == "" {
		account.KeyBook = st.Origin.Header().KeyBook
	} else {
		keyBookUrl, err := url.Parse(body.KeyBookUrl)
		if err != nil {
			return nil, fmt.Errorf("invalid key book URL: %v", err)
		}

		book := new(protocol.KeyBook)
		err = st.LoadUrlAs(keyBookUrl, book)
		if err != nil {
			return nil, fmt.Errorf("invalid key book %q: %v", keyBookUrl, err)
		}

		account.KeyBook = types.String(keyBookUrl.String())
	}

	st.Create(account)
	return nil, nil
}
