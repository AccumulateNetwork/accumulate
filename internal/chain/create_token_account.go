package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateTokenAccount struct{}

func (CreateTokenAccount) Type() types.TxType { return types.TxTypeCreateTokenAccount }

func (CreateTokenAccount) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateTokenAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateTokenAccount), tx.Transaction.Body)
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
	account.Url = accountUrl.String()
	account.TokenUrl = tokenUrl.String()
	account.Scratch = body.Scratch
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

		account.KeyBook = keyBookUrl.String()
	}
	if body.Manager != "" {
		account.ManagerKeyBook = body.Manager
	}

	st.Create(account)
	return nil, nil
}
