package chain

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
)

type TokenAccountCreate struct{}

func (TokenAccountCreate) Type() types.TxType { return types.TxTypeTokenAccountCreate }

func checkTokenAccountCreate(st *state.StateEntry, tx *transactions.GenTransaction) (accountUrl, tokenUrl *url.URL, err error) {
	if st.ChainHeader == nil {
		return nil, nil, fmt.Errorf("sponsor not found")
	}

	sponsorUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	body := new(api.TokenAccount)
	err = tx.As(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err = url.Parse(*body.URL.AsString())
	if err != nil {
		return nil, nil, fmt.Errorf("invalid account URL: %v", err)
	}

	tokenUrl, err = url.Parse(*body.TokenURL.AsString())
	if err != nil {
		return nil, nil, fmt.Errorf("invalid token URL: %v", err)
	}
	// TODO Make sure tokenUrl is a real kind of token

	if !bytes.Equal(accountUrl.IdentityChain(), sponsorUrl.IdentityChain()) {
		return nil, nil, fmt.Errorf("%q cannot sponsor %q", sponsorUrl.String(), accountUrl.String())
	}

	return accountUrl, tokenUrl, nil
}

func (TokenAccountCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, err := checkTokenAccountCreate(st, tx)
	return err
}

func (TokenAccountCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	accountUrl, tokenUrl, err := checkTokenAccountCreate(st, tx)
	if err != nil {
		return nil, err
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	err = scc.Add(state.NewTokenAccount(accountUrl.String(), tokenUrl.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	syn := new(transactions.GenTransaction)
	syn.SigInfo = new(transactions.SignatureInfo)
	syn.SigInfo.URL = accountUrl.String()
	syn.Transaction, err = scc.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction")
	}

	res := new(DeliverTxResult)
	res.AddSyntheticTransaction(syn)
	return res, nil
}
