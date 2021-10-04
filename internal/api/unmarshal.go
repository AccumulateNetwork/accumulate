package api

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	tm "github.com/tendermint/tendermint/abci/types"
)

func unmarshalAs(rQuery tm.ResponseQuery, typ string, as func([]byte) (interface{}, error)) (*api.APIDataResponse, error) {
	rAPI := new(api.APIDataResponse)
	rAPI.Type = types.String(typ)

	if rQuery.Code != 0 {
		data, err := json.Marshal(rQuery.Value)
		if err != nil {
			return nil, err
		}

		rAPI.Data = (*json.RawMessage)(&data)
		return rAPI, nil
	}

	v, err := as(rQuery.Value)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	rAPI.Data = (*json.RawMessage)(&data)
	return rAPI, nil
}

func unmarshalADI(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "adi", func(b []byte) (interface{}, error) {
		adi := new(response.ADI)
		err := adi.ADI.UnmarshalBinary(b)
		return adi, err
	})
}

func unmarshalToken(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "token", func(b []byte) (interface{}, error) {
		sToken := new(state.Token)
		err := sToken.UnmarshalBinary(b)
		rToken := new(response.Token)
		rToken.Precision = sToken.Precision
		rToken.URL = sToken.ChainUrl
		rToken.Symbol = sToken.Symbol
		rToken.Meta = sToken.Meta
		return rToken, err
	})
}

func unmarshalTokenAccount(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "tokenAccount", func(b []byte) (interface{}, error) {
		sAccount := new(state.TokenAccount)
		err := sAccount.UnmarshalBinary(b)
		ta := api.NewTokenAccount(sAccount.ChainUrl, sAccount.TokenUrl.String)
		rAccount := response.NewTokenAccount(ta, sAccount.GetBalance())
		return rAccount, err
	})
}

func unmarshalTokenTx(rQuery tm.ResponseQuery) (*transactions.TokenSend, error) {
	_, txRaw := common.BytesSlice(rQuery.Value)
	sTx := new(state.Transaction)
	err := sTx.UnmarshalBinary(txRaw)
	if err != nil {
		return nil, NewAccumulateError(err)
	}

	tx := new(transactions.TokenSend)
	_, err = tx.Unmarshal(sTx.Transaction.Bytes())
	if err != nil {
		return nil, NewAccumulateError(err)
	}

	return tx, nil
}

func (q *Query) unmarshalByType(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	sChain := new(state.Chain)
	err := sChain.UnmarshalBinary(rQuery.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid state object: %v", err)
	}

	switch sChain.Type {
	case types.ChainTypeAdi:
		return unmarshalADI(rQuery)

	case types.ChainTypeToken:
		return unmarshalToken(rQuery)

	case types.ChainTypeTokenAccount, types.ChainTypeSignatureGroup:
		// TODO Is it really OK to unmarshal a sig group as an account? That's
		// what the orginal `ChainStates` did...
		return unmarshalTokenAccount(rQuery)

	case types.ChainTypeAnonTokenAccount:
		adi, _, _ := types.ParseIdentityChainPath(sChain.ChainUrl.AsString())
		adi += "/dc/ACME"
		return q.GetTokenAccount(&adi)
	}

	rAPI := new(api.APIDataResponse)
	rAPI.Type = types.String(sChain.Type.Name())
	msg := []byte(fmt.Sprintf("{\"entry\":\"%x\"}", rQuery.Value))
	rAPI.Data = (*json.RawMessage)(&msg)
	return rAPI, nil
}
