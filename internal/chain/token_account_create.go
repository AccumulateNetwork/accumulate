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

func checkTokenAccountCreate(st *StateManager, tx *transactions.GenTransaction) (accountUrl, tokenUrl *url.URL, err error) {
	body := new(protocol.TokenAccountCreate)
	err = tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err = url.Parse(body.Url)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid account URL: %v", err)
	}

	tokenUrl, err = url.Parse(body.TokenUrl)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid token URL: %v", err)
	}
	// TODO Make sure tokenUrl is a real kind of token

	if !accountUrl.Identity().Equal(st.SponsorUrl) {
		return nil, nil, fmt.Errorf("%q cannot sponsor %q", st.SponsorUrl, accountUrl)
	}

	return accountUrl, tokenUrl, nil
}

func (TokenAccountCreate) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, err := checkTokenAccountCreate(st, tx)
	return err
}

func (TokenAccountCreate) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	accountUrl, tokenUrl, err := checkTokenAccountCreate(st, tx)
	if err != nil {
		return err
	}

	account := state.NewTokenAccount(accountUrl.String(), tokenUrl.String())

	// TODO Allow the token account to be overridden
	account.SigSpecId = st.Sponsor.Header().SigSpecId

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	err = scc.Add(account)
	if err != nil {
		return fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	st.Submit(accountUrl, scc)
	return nil
}
