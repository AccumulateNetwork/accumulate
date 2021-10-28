package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TokenAccountCreate struct{}

func (TokenAccountCreate) Type() types.TxType { return types.TxTypeTokenAccountCreate }

func checkTokenAccountCreate(st *StateManager, tx *transactions.GenTransaction) (accountUrl, tokenUrl, keyBookUrl *url.URL, err error) {
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

func (TokenAccountCreate) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, err := checkTokenAccountCreate(st, tx)
	return err
}

func (TokenAccountCreate) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	accountUrl, tokenUrl, keyBookUrl, err := checkTokenAccountCreate(st, tx)
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
