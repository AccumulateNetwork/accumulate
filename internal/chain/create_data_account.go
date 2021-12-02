package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types/state"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateDataAccount struct{}

func (CreateDataAccount) Type() types.TxType { return types.TxTypeCreateDataAccount }

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

	if !dataAccountUrl.Identity().Equal(st.SponsorUrl) {
		return fmt.Errorf("%q cannot sponsor %q", st.SponsorUrl, dataAccountUrl)
	}

	state.NewTokenAccount()
	account := state.NewDataAccount(dataAccountUrl.String(), tokenUrl.String())
	if body.KeyBookUrl == "" {
		account.SigSpecId = st.Sponsor.Header().SigSpecId
	} else {
		keyBookUrl, err := url.Parse(body.KeyBookUrl)
		if err != nil {
			return fmt.Errorf("invalid key book URL: %v", err)
		}

		ssg := new(protocol.SigSpecGroup)
		err = st.LoadUrlAs(keyBookUrl, ssg)
		if err != nil {
			return fmt.Errorf("invalid key book %q: %v", keyBookUrl, err)
		}

		copy(account.SigSpecId[:], keyBookUrl.ResourceChain())
	}

	st.Create(account)
	return errors.New("not implemented") // TODO
}
