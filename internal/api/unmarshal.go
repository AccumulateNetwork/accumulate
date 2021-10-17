package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
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
		sAdi := new(state.AdiState)
		err := sAdi.UnmarshalBinary(b)
		rAdi := new(response.ADI)
		rAdi.URL = sAdi.ChainUrl
		rAdi.PublicKeyHash = sAdi.KeyData.AsBytes32()
		return rAdi, err
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
		rAccount := response.NewTokenAccount(ta, sAccount.GetBalance(), sAccount.TxCount)
		return rAccount, err
	})
}

func unmarshalAnonTokenAccount(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "anonTokenAccount", func(b []byte) (interface{}, error) {
		sAccount := new(protocol.AnonTokenAccount)
		err := sAccount.UnmarshalBinary(b)
		rAccount := new(response.AnonTokenAccount)
		rAccount.TokenAccount = api.NewTokenAccount(sAccount.ChainUrl, types.String(sAccount.TokenUrl))
		rAccount.Balance = types.Amount{Int: sAccount.Balance}
		rAccount.CreditBalance = types.Amount{Int: sAccount.CreditBalance}
		rAccount.TxCount = sAccount.TxCount
		rAccount.Nonce = sAccount.Nonce
		return rAccount, err
	})
}

func unmarshalSigSpec(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "sigSpec", func(b []byte) (interface{}, error) {
		r := new(protocol.SigSpec)
		err := r.UnmarshalBinary(b)
		return r, err
	})
}

func unmarshalSigSpecGroup(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "sigSpecGroup", func(b []byte) (interface{}, error) {
		r := new(protocol.SigSpecGroup)
		err := r.UnmarshalBinary(b)
		return r, err
	})
}

func unmarshalTxReference(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "txReference", func(b []byte) (interface{}, error) {
		txRef := new(state.TxReference)
		err := txRef.UnmarshalBinary(b)
		txRefResp := response.TxReference{TxId: txRef.TxId}
		return txRefResp, err
	})
}

func unmarshalTokenTx(txPayload []byte, txId types.Bytes, txSynthTxIds types.Bytes) (*api.APIDataResponse, error) {
	tx := api.TokenTx{}
	err := tx.UnmarshalBinary(txPayload)
	if err != nil {
		return nil, NewAccumulateError(err)
	}
	txResp := response.TokenTx{}
	txResp.From = tx.From.String
	txResp.TxId = txId

	if len(txSynthTxIds)/32 != len(tx.To) {
		return nil, fmt.Errorf("number of synthetic tx, does not match number of outputs")
	}

	//should receive tx,unmarshal to output accounts
	for i, v := range tx.To {
		j := i * 32
		synthTxId := txSynthTxIds[j : j+32]
		txStatus := response.TokenTxOutputStatus{}
		txStatus.TokenTxOutput.URL = v.URL
		txStatus.TokenTxOutput.Amount = v.Amount
		txStatus.SyntheticTxId = synthTxId

		txResp.ToAccount = append(txResp.ToAccount, txStatus)
	}

	data, err := json.Marshal(&txResp)
	if err != nil {
		return nil, err
	}
	resp := api.APIDataResponse{}
	resp.Type = types.String(types.TxTypeTokenTx.Name())
	resp.Data = new(json.RawMessage)
	*resp.Data = data
	return &resp, err
}

//unmarshalSynthTokenDeposit will unpack the synthetic token deposit and pack it into the response
func unmarshalSynthTokenDeposit(txPayload []byte, txId types.Bytes, txSynthTxIds types.Bytes) (*api.APIDataResponse, error) {
	_ = txId
	tx := synthetic.TokenTransactionDeposit{}
	err := tx.UnmarshalBinary(txPayload)
	if err != nil {
		return nil, NewAccumulateError(err)
	}

	if len(txSynthTxIds) != 0 {
		return nil, fmt.Errorf("there should be no synthetic transaction associated with this transaction")
	}

	data, err := json.Marshal(&tx)
	if err != nil {
		return nil, err
	}
	resp := api.APIDataResponse{}
	resp.Type = types.String(types.TxTypeSyntheticTokenDeposit.Name())
	resp.Data = new(json.RawMessage)
	*resp.Data = data
	return &resp, err
}

//unmarshalTransaction will unpack the transaction stored on-chain and marshal it into a response
func unmarshalTransaction(txPayload []byte, txId []byte, txSynthTxIds []byte) (resp *api.APIDataResponse, err error) {

	txType, _ := common.BytesUint64(txPayload)
	switch types.TxType(txType) {
	case types.TxTypeTokenTx:
		resp, err = unmarshalTokenTx(txPayload, txId, txSynthTxIds)
	case types.TxTypeSyntheticTokenDeposit:
		resp, err = unmarshalSynthTokenDeposit(txPayload, txId, txSynthTxIds)
	default:
		err = fmt.Errorf("unable to extract transaction info for type %s : %x", types.TxType(txType).Name(), txPayload)
	}

	return resp, err
}

func unmarshalChainState(rQuery tm.ResponseQuery, expect ...types.ChainType) (*api.APIDataResponse, error) {
	sChain := new(state.ChainHeader)
	err := sChain.UnmarshalBinary(rQuery.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid state object: %v", err)
	}

	if len(expect) > 0 {
		if err := isExpected(expect, sChain.Type); err != nil {
			return nil, err
		}
	}

	switch sChain.Type {
	case types.ChainTypeAdi:
		return unmarshalADI(rQuery)

	case types.ChainTypeToken:
		return unmarshalToken(rQuery)

	case types.ChainTypeTokenAccount:
		return unmarshalTokenAccount(rQuery)

	case types.ChainTypeAnonTokenAccount:
		return unmarshalAnonTokenAccount(rQuery)

	case types.ChainTypeSigSpec:
		return unmarshalSigSpec(rQuery)

	case types.ChainTypeSigSpecGroup:
		return unmarshalSigSpecGroup(rQuery)
	}

	rAPI := new(api.APIDataResponse)
	rAPI.Type = types.String(sChain.Type.Name())
	msg := []byte(fmt.Sprintf("{\"entry\":\"%x\"}", rQuery.Value))
	rAPI.Data = (*json.RawMessage)(&msg)
	return rAPI, nil
}

func isExpected(expect []types.ChainType, typ types.ChainType) error {
	for _, e := range expect {
		if e == typ {
			return nil
		}
	}

	if len(expect) == 1 {
		return fmt.Errorf("want %v, got %v", expect[0], typ)
	}

	s := make([]string, len(expect))
	for i, e := range expect {
		s[i] = e.String()
	}
	return fmt.Errorf("want one of %s; got %v", strings.Join(s, ", "), typ)
}
