package chain

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type CreateDataAccount struct{}

func (CreateDataAccount) Type() types.TransactionType { return types.TxTypeCreateDataAccount }

func (CreateDataAccount) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.CreateDataAccount)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	dataAccountUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid account URL: %v", err)
	}

	//only the ADI can create the data account associated with the ADI
	if !dataAccountUrl.Identity().Equal(st.SponsorUrl) {
		return fmt.Errorf("%q cannot sponsor %q", st.SponsorUrl, dataAccountUrl)
	}

	//if we have a manger book URL, then we need to parse it.
	var managerBookUrl string
	if len(body.ManagerKeyBookUrl) != 0 {
		u, err := url.Parse(body.ManagerKeyBookUrl)
		if err != nil {
			return fmt.Errorf("manager key book specified, but url is invalid: %v", err)
		}
		managerBookUrl = u.String()
	}

	//create the data account
	account := state.NewDataAccount(dataAccountUrl.String(), managerBookUrl)

	//setup key book associated with account
	if body.KeyBookUrl == "" {
		account.KeyBook = st.Sponsor.Header().KeyBook
	} else {
		keyBookUrl, err := url.Parse(body.KeyBookUrl)
		if err != nil {
			return fmt.Errorf("invalid key book URL: %v", err)
		}

		ssg := new(protocol.KeyBook)
		err = st.LoadUrlAs(keyBookUrl, ssg)
		if err != nil {
			return fmt.Errorf("invalid key book %q: %v", keyBookUrl, err)
		}
		copy(account.KeyBook[:], keyBookUrl.ResourceChain())
	}

	if managerBookUrl != "" {
		c, err := st.LoadString(managerBookUrl)
		if err != nil {
			return fmt.Errorf("invalid key book %q: %v", managerBookUrl, err)
		}
		if c.Header().Type != types.ChainTypeKeyBook {
			return fmt.Errorf("expected key book url for %v, received %s",
				managerBookUrl, c.Header().Type)
		}
	}

	st.Create(account)
	return nil
}
