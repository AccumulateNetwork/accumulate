package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type CreateTokenAccount struct{}

func (CreateTokenAccount) Type() types.TxType { return types.TxTypeCreateTokenAccount }

func (CreateTokenAccount) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	body := new(protocol.TokenAccountCreate)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid account URL: %v", err)
	}

	tokenUrl, err := url.Parse(body.TokenUrl)
	if err != nil {
		return fmt.Errorf("invalid token URL: %v", err)
	}
	// TODO Make sure tokenUrl is a real kind of token

	if !accountUrl.Identity().Equal(st.SponsorUrl) {
		return fmt.Errorf("%q cannot sponsor %q", st.SponsorUrl, accountUrl)
	}

	account := state.NewTokenAccount(accountUrl.String(), tokenUrl.String())
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
	return nil
}
