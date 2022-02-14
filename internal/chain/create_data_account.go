package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateDataAccount struct{}

func (CreateDataAccount) Type() types.TransactionType { return types.TxTypeCreateDataAccount }

func (CreateDataAccount) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.CreateDataAccount)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateDataAccount), tx.Transaction.Body)
	}

	dataAccountUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %v", err)
	}

	//only the ADI can create the data account associated with the ADI
	if !dataAccountUrl.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q cannot be the origininator of %q", st.OriginUrl, dataAccountUrl)
	}

	//create the data account
	account := protocol.NewDataAccount()
	account.Url = dataAccountUrl.String()
	account.Scratch = body.Scratch

	//if we have a manger book URL, then we need make sure it is valid url syntax
	if body.ManagerKeyBookUrl != "" {
		u, err := url.Parse(body.ManagerKeyBookUrl)
		if err != nil {
			return nil, fmt.Errorf("manager key book specified, but url is invalid: %v", err)
		}
		account.ManagerKeyBook = u.String()
	}

	//setup key book associated with account
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

	st.Create(account)
	return nil, nil
}
