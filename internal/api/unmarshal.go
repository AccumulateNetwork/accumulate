package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/response"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	tm "github.com/tendermint/tendermint/abci/types"
)

func responseIsError(rQuery tm.ResponseQuery) error {
	if rQuery.Code == 0 {
		return nil
	}

	switch {
	case rQuery.Code == protocol.CodeNotFound:
		return storage.ErrNotFound
	case rQuery.Log != "":
		return errors.New(rQuery.Log)
	case rQuery.Info != "":
		return errors.New(rQuery.Info)
	default:
		return fmt.Errorf("query failed with code %d", rQuery.Code)
	}
}

func unmarshalAs(rQuery tm.ResponseQuery, typ string, as func([]byte) (interface{}, error)) (*api.APIDataResponse, error) {
	if err := responseIsError(rQuery); err != nil {
		return nil, err
	}

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

	obj := state.Object{}
	err := obj.UnmarshalBinary(rQuery.Value)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling state object %v", err)
	}

	v, err := as(obj.Entry)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	rAPI.MerkleState = new(api.MerkleState)
	rAPI.MerkleState.Count = obj.Height
	rAPI.MerkleState.Roots = make([]types.Bytes, len(obj.Roots))
	for i, r := range obj.Roots {
		rAPI.MerkleState.Roots[i] = r
	}
	rAPI.Data = (*json.RawMessage)(&data)
	return rAPI, nil
}

func unmarshalADI(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "adi", func(b []byte) (interface{}, error) {
		sAdi := new(state.AdiState)
		err := sAdi.UnmarshalBinary(b)
		rAdi := new(response.ADI)
		rAdi.Url = *sAdi.ChainUrl.AsString()
		rAdi.PublicKey = sAdi.KeyData
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
		rToken.PropertiesUrl = sToken.PropertiesUrl
		return rToken, err
	})
}

func unmarshalTokenAccount(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "tokenAccount", func(b []byte) (interface{}, error) {
		sAccount := new(state.TokenAccount)
		err := sAccount.UnmarshalBinary(b)
		ta := new(protocol.TokenAccountCreate)
		ta.Url = string(sAccount.ChainUrl)
		ta.TokenUrl = string(sAccount.TokenUrl.String)
		rAccount := response.NewTokenAccount(ta, sAccount.GetBalance(), sAccount.TxCount)
		return rAccount, err
	})
}

func unmarshalAnonTokenAccount(rQuery tm.ResponseQuery) (*api.APIDataResponse, error) {
	return unmarshalAs(rQuery, "anonTokenAccount", func(b []byte) (interface{}, error) {
		sAccount := new(protocol.AnonTokenAccount)
		err := sAccount.UnmarshalBinary(b)
		rAccount := new(response.AnonTokenAccount)
		rAccount.TokenAccountCreate = new(protocol.TokenAccountCreate)
		rAccount.Url = string(sAccount.ChainUrl)
		rAccount.TokenUrl = string(sAccount.TokenUrl)
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
		obj := state.Object{}
		err := obj.UnmarshalBinary(b)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling Token state object %v", err)
		}

		txRef := new(state.TxReference)
		err = txRef.UnmarshalBinary(obj.Entry)
		txRefResp := response.TxReference{TxId: txRef.TxId}
		return txRefResp, err
	})
}

func unmarshalTokenTx(txPayload []byte, txId types.Bytes, txSynthTxIds types.Bytes) (*api.APIDataResponse, error) {
	tx := api.TokenTx{}
	err := tx.UnmarshalBinary(txPayload)
	if err != nil {
		return nil, accumulateError(err)
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
	resp.Type = types.String(types.TxTypeWithdrawTokens.Name())
	resp.Data = new(json.RawMessage)
	*resp.Data = data
	resp.Sponsor = tx.From.String
	return &resp, err
}

//unmarshalSynthTokenDeposit will unpack the synthetic token deposit and pack it into the response
func unmarshalSynthTokenDeposit(txPayload []byte, _ types.Bytes, txSynthTxIds types.Bytes) (*api.APIDataResponse, error) {
	if len(txSynthTxIds) != 0 {
		return nil, fmt.Errorf("there should be no synthetic transaction associated with this transaction")
	}

	tx := new(synthetic.TokenTransactionDeposit)
	resp, err := unmarshalTxAs(txPayload, tx)
	if err != nil {
		return nil, err
	}

	resp.Sponsor = tx.FromUrl
	return resp, err
}

func unmarshalTxAs(payload []byte, v protocol.TransactionPayload) (*api.APIDataResponse, error) {
	err := v.UnmarshalBinary(payload)
	if err != nil {
		return nil, accumulateError(err)
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	resp := api.APIDataResponse{}
	resp.Type = types.String(v.GetType().Name())
	resp.Data = (*json.RawMessage)(&data)
	return &resp, err
}

//unmarshalTransaction will unpack the transaction stored on-chain and marshal it into a response
func unmarshalTransaction(sigInfo *transactions.SignatureInfo, txPayload []byte, txId []byte, txSynthTxIds []byte) (resp *api.APIDataResponse, err error) {

	txType, _ := common.BytesUint64(txPayload)
	switch types.TxType(txType) {
	case types.TxTypeWithdrawTokens:
		resp, err = unmarshalTokenTx(txPayload, txId, txSynthTxIds)
	case types.TxTypeSyntheticDepositTokens:
		resp, err = unmarshalSynthTokenDeposit(txPayload, txId, txSynthTxIds)
	case types.TxTypeCreateIdentity:
		resp, err = unmarshalTxAs(txPayload, new(protocol.IdentityCreate))
	case types.TxTypeCreateTokenAccount:
		resp, err = unmarshalTxAs(txPayload, new(protocol.TokenAccountCreate))
	case types.TxTypeCreateKeyPage:
		resp, err = unmarshalTxAs(txPayload, new(protocol.CreateSigSpec))
	case types.TxTypeCreateKeyBook:
		resp, err = unmarshalTxAs(txPayload, new(protocol.CreateSigSpecGroup))
	case types.TxTypeAddCredits:
		resp, err = unmarshalTxAs(txPayload, new(protocol.AddCredits))
	case types.TxTypeUpdateKeyPage:
		resp, err = unmarshalTxAs(txPayload, new(protocol.UpdateKeyPage))
	case types.TxTypeSyntheticCreateChain:
		resp, err = unmarshalTxAs(txPayload, new(protocol.SyntheticCreateChain))
	case types.TxTypeSyntheticDepositCredits:
		resp, err = unmarshalTxAs(txPayload, new(protocol.SyntheticDepositCredits))
	case types.TxTypeSyntheticGenesis:
		resp, err = unmarshalTxAs(txPayload, new(protocol.SyntheticGenesis))
	case types.TxTypeAcmeFaucet:
		resp, err = unmarshalTxAs(txPayload, new(protocol.AcmeFaucet))
	default:
		err = fmt.Errorf("unable to extract transaction info for type %s : %x", types.TxType(txType).Name(), txPayload)
	}
	if err != nil {
		return nil, err
	}

	if resp.Sponsor == "" {
		resp.Sponsor = types.String(sigInfo.URL)
	}
	return resp, err
}

func unmarshalQueryResponse(rQuery tm.ResponseQuery, expect ...types.ChainType) (*api.APIDataResponse, error) {
	if err := responseIsError(rQuery); err != nil {
		return nil, err
	}

	switch typ := string(rQuery.Key); typ {
	case "tx":
		rid := query.ResponseByTxId{}
		err := rid.UnmarshalBinary(rQuery.Value)
		if err != nil {
			return nil, err
		}

		return packTransactionQuery(rid.TxId[:], rid.TxState, rid.TxPendingState, rid.TxSynthTxIds)
	case "chain":
		// OK
	default:
		return nil, fmt.Errorf("want tx or chain, got %q", typ)
	}

	obj := state.Object{}
	err := obj.UnmarshalBinary(rQuery.Value)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling chain state object %v", err)
	}

	sChain := new(state.ChainHeader)
	err = sChain.UnmarshalBinary(obj.Entry)
	if err != nil {
		return nil, fmt.Errorf("invalid state object: %v", err)
	}

	if len(expect) > 0 {
		if err := isExpected(expect, sChain.Type); err != nil {
			return nil, err
		}
	}

	switch sChain.Type {
	case types.ChainTypeIdentity:
		return unmarshalADI(rQuery)

	case types.ChainTypeTokenIssuer:
		return unmarshalToken(rQuery)

	case types.ChainTypeTokenAccount:
		return unmarshalTokenAccount(rQuery)

	case types.ChainTypeLiteTokenAccount:
		return unmarshalAnonTokenAccount(rQuery)

	case types.ChainTypeKeyPage:
		return unmarshalSigSpec(rQuery)

	case types.ChainTypeKeyBook:
		return unmarshalSigSpecGroup(rQuery)

	case types.ChainTypeTransaction:
		return unmarshalAs(rQuery, "tx", func(b []byte) (interface{}, error) {
			r := new(state.Transaction)
			err := r.UnmarshalBinary(b)
			return r, err
		})
	}

	rAPI := new(api.APIDataResponse)
	rAPI.Type = types.String(sChain.Type.Name())

	msg := []byte(fmt.Sprintf("{\"state\":\"%x\"}", obj.Entry))
	rAPI.MerkleState = new(api.MerkleState)
	rAPI.MerkleState.Count = obj.Height
	rAPI.MerkleState.Roots = make([]types.Bytes, len(obj.Roots))
	for i, r := range obj.Roots {
		rAPI.MerkleState.Roots[i] = r
	}
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
