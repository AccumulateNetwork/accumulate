package chain

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type TokenAccountCreate struct{}

func (TokenAccountCreate) Type() types.TxType { return types.TxTypeTokenAccountCreate }

func (TokenAccountCreate) BeginBlock() {}

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

	synth := new(synthetic.TokenAccountCreate)
	synth.SetHeader(tx.TransactionHash(), (*types.String)(&tx.SigInfo.URL), &tcc.URL)
	synth.TokenURL = tcc.TokenURL

	stx := new(transactions.GenTransaction)
	stx.SigInfo = new(transactions.SignatureInfo)
	stx.SigInfo.URL = *tcc.URL.AsString()
	stx.Transaction, err = synth.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction")
	}

	res := new(DeliverTxResult)
	res.AddSyntheticTransaction(stx)
	return res, nil
}
