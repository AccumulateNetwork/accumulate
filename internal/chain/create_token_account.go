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

func checkCreateTokenAccount(st *StateManager, tx *transactions.GenTransaction) (accountUrl, tokenUrl, keyBookUrl *url.URL, err error) {
	body := new(protocol.TokenAccountCreate)
	err = tx.As(body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err = url.Parse(body.Url)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid account URL: %v", err)
	}

	tokenUrl, err = url.Parse(body.TokenUrl)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid token URL: %v", err)
	}
	// TODO Make sure tokenUrl is a real kind of token

	if !accountUrl.Identity().Equal(st.SponsorUrl) {
		return nil, nil, nil, fmt.Errorf("%q cannot sponsor %q", st.SponsorUrl, accountUrl)
	}

	if body.KeyBookUrl != "" {
		keyBookUrl, err = url.Parse(body.KeyBookUrl)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid key book URL: %v", err)
		}

		ssg := new(protocol.SigSpecGroup)
		err = st.LoadUrlAs(keyBookUrl, ssg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid key book %q: %v", keyBookUrl, err)
		}
	}

	return accountUrl, tokenUrl, keyBookUrl, nil
}

func (CreateTokenAccount) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, err := checkCreateTokenAccount(st, tx)
	return err
}

func (CreateTokenAccount) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	accountUrl, tokenUrl, keyBookUrl, err := checkCreateTokenAccount(st, tx)
	if err != nil {
		return err
	}

	account := state.NewTokenAccount(accountUrl.String(), tokenUrl.String())

	if keyBookUrl == nil {
		account.SigSpecId = st.Sponsor.Header().SigSpecId
	} else {
		copy(account.SigSpecId[:], keyBookUrl.ResourceChain())
	}

	st.Create(account)
	return nil
}
