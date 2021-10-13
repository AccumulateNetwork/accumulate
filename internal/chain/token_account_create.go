package chain

import (
	"bytes"
	"encoding/json"
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

func (TokenAccountCreate) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	if st == nil {
		return fmt.Errorf("current state not defined")
	}

	if st.AdiState == nil {
		return fmt.Errorf("identity not defined")
	}

	if st.ChainState != nil {
		return fmt.Errorf("chain already defined")
	}
	tcc := api.TokenAccount{}
	err := json.Unmarshal(tx.Transaction, &tcc)
	if err != nil {
		return fmt.Errorf("data payload of submission is not a valid token chain create message")
	}

	return nil
}

func (TokenAccountCreate) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if st == nil {
		return nil, fmt.Errorf("current state not defined")
	}
	if st.ChainHeader == nil {
		return nil, fmt.Errorf("sponsor does not exist")
	}

	tcc := new(api.TokenAccount)
	err := tx.As(tcc)
	if err != nil {
		return nil, fmt.Errorf("data payload of submission is not a valid token chain create message")
	}

	sponsorUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	acctUrl, err := url.Parse(*tcc.URL.AsString())
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %v", err)
	}

	tokenUrl, err := url.Parse(*tcc.TokenURL.AsString())
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}
	// TODO Make sure tokenUrl is a real kind of token

	if !bytes.Equal(acctUrl.IdentityChain(), sponsorUrl.IdentityChain()) {
		return nil, fmt.Errorf("%q cannot sponsor %q", sponsorUrl.String(), acctUrl.String())
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	err = scc.Add(state.NewTokenAccount(acctUrl.String(), tokenUrl.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic TX: %v", err)
	}

	syn := new(transactions.GenTransaction)
	syn.SigInfo = new(transactions.SignatureInfo)
	syn.SigInfo.URL = *tcc.URL.AsString()
	syn.Transaction, err = scc.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction")
	}

	res := new(DeliverTxResult)
	res.AddSyntheticTransaction(syn)
	return res, nil
}
