package router

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeApi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

type Query struct {
	txBouncer *relay.Relay
}

func NewQuery(txBouncer *relay.Relay) *Query {
	q := Query{}
	//q.client = abcicli.NewLocalClient(nil, app)
	q.txBouncer = txBouncer
	return &q
}

//"GetADI()"
//"GetToken()"
//"GetTokenAccount()"
//"GetTokenTx()"
//"GetData()" Submit url into, and receive ADI/Token/TokenAccount/TokenTx

// GetAdi get the adi state object. Use this to get the nonce.
func (q *Query) GetAdi(adi *string) (*acmeApi.APIDataResponse, error) {

	var err error
	if err != nil {
		return nil, fmt.Errorf("cannot marshal query for Token issuance")
	}

	aResp, err := q.txBouncer.Query(adi, nil)

	if err != nil {
		return nil, fmt.Errorf("bvc adi query returned error, %v", err)
	}

	qResp := aResp.Response

	if err != nil {
		return nil, fmt.Errorf("bvc adi query returned error, %v", err)
	}

	//unpack the response
	ret := &acmeApi.APIDataResponse{}
	ret.Type = "adi"

	if qResp.Code == 0 {
		//unpack the state object returned from the query
		adiResp := response.ADI{}
		err = adiResp.ADI.UnmarshalBinary(qResp.Value)

		//package the response data into json
		var data json.RawMessage
		data, err = json.Marshal(adiResp)
		if err != nil {
			return nil, fmt.Errorf("cannot extract token information")
		}

		ret.Data = &data
	} else {
		var data json.RawMessage
		data, err = json.Marshal(qResp.Value)
		ret.Data = &data
	}

	return ret, err
}

// GetToken
// retrieve the informatin regarding a token
func (q *Query) GetToken(tokenUrl *string) (*acmeApi.APIDataResponse, error) {

	var err error
	aResp, err := q.txBouncer.Query(tokenUrl, nil)

	if err != nil {
		return nil, fmt.Errorf("bvc token query returned error, %v", err)
	}
	qResp := aResp.Response

	//unpack the response

	ret := &acmeApi.APIDataResponse{}
	ret.Type = "token"

	if qResp.Code == 0 {

		//unpack the state object returned from the query
		tokState := &state.Token{}
		err = tokState.UnmarshalBinary(qResp.Value)

		tokResp := response.Token{}

		tokResp.Precision = tokState.Precision
		tokResp.URL = tokState.ChainUrl
		tokResp.Symbol = tokState.Symbol
		tokResp.Meta = tokState.Meta
		//package the response data into json
		var data json.RawMessage
		data, err = json.Marshal(tokResp)
		if err != nil {
			return nil, fmt.Errorf("cannot extract token information")
		}

		ret.Data = &data
	} else {
		var data json.RawMessage
		data, err = json.Marshal(qResp.Value)
		ret.Data = &data
	}

	return ret, err
}

// GetTokenAccount get the token balance for a given url
func (q *Query) GetTokenAccount(adiChainPath *string) (*acmeApi.APIDataResponse, error) {

	var err error

	aResp, err := q.txBouncer.Query(adiChainPath, nil)

	if err != nil {
		return nil, fmt.Errorf("bvc token account query returned error, %v", err)
	}
	qResp := aResp.Response

	ret := &acmeApi.APIDataResponse{}
	ret.Type = "tokenAccount"

	if qResp.Code == 0 {

		//unpack the state object returned from the query
		tokState := &state.TokenAccount{}
		err = tokState.UnmarshalBinary(qResp.Value)

		ta := acmeApi.NewTokenAccount(tokState.ChainUrl, tokState.TokenUrl.String)
		tokResp := response.NewTokenAccount(ta, tokState.GetBalance())
		//package the response data into json
		var data json.RawMessage
		data, err = json.Marshal(tokResp)
		if err != nil {
			return nil, fmt.Errorf("cannot extract token information")
		}

		ret.Data = &data
	} else {
		var data json.RawMessage
		data, err = json.Marshal(qResp.Value)
		ret.Data = &data
	}

	return ret, err
}

// GetTokenTx
// get the token tx from the primary adi, then query all the output accounts to get the status
func (q *Query) GetTokenTx(tokenAccountUrl *string, txId []byte) (resp interface{}, err error) {
	// need to know the ADI and ChainID, deriving adi and chain id from TokenTx.From

	aResp, err := q.txBouncer.Query(tokenAccountUrl, txId)

	if err != nil {
		return nil, fmt.Errorf("bvc token tx query returned error, %v", err)
	}
	qResp := aResp.Response

	data, txRaw := common.BytesSlice(qResp.Value)
	data, txPending := common.BytesSlice(data)
	_ = txPending
	txState := state.Transaction{}
	err = txState.UnmarshalBinary(txRaw)
	if err != nil {
		return resp, NewAccumulateError(err)
	}
	//now unmarshal the token transaction on-chain
	//need to identify type of TX, for now only support token tx
	tx := transactions.TokenSend{}
	_, err = tx.Unmarshal(txState.Transaction.Bytes())
	if err != nil {
		return resp, NewAccumulateError(err)
	}
	txResp := response.TokenTx{}
	txResp.FromUrl = types.String(*tokenAccountUrl)
	copy(txResp.TxId[:], txId)

	//should receive tx,unmarshal to output accounts
	for _, v := range tx.Outputs {
		aResp, err := q.txBouncer.Query(tokenAccountUrl, txId)

		txStatus := response.TokenTxAccountStatus{}
		if err != nil {
			txStatus.Status = types.String(fmt.Sprintf("transaction not found for %s, %v", v.Dest, err))
			txStatus.AccountUrl = types.String(v.Dest)
		} else {
			qResp = aResp.Response
			err = txStatus.UnmarshalBinary(qResp.Value)
			if err != nil {
				txStatus.Status = types.String(fmt.Sprintf("%v", err))
				txStatus.AccountUrl = types.String(v.Dest)
			}
		}

		txResp.ToAccount = append(txResp.ToAccount, txStatus)
	}

	return txResp, err
}

var ChainStates = map[types.ChainType]interface{}{
	types.ChainTypeAdi: func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetAdi(url)
	},
	types.ChainTypeToken: func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetToken(url)
	},
	types.ChainTypeTokenAccount: func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetTokenAccount(url)
	},
	types.ChainTypeSignatureGroup: func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetTokenAccount(url)
	},
	types.ChainTypeAnonTokenAccount: func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		adi, _, _ := types.ParseIdentityChainPath(url)
		adi += "/dc/ACME"
		return q.GetTokenAccount(&adi)
	},
}

// GetChainState
// will return the state object of the chain, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainState(adiChainPath *string, txId []byte) (interface{}, error) {
	var err error

	var qResp *tmtypes.ResponseQuery

	//this QuerySync call is only temporary until we get router setup.
	aResp, err := q.txBouncer.Query(adiChainPath, txId)
	if err != nil {
		return nil, fmt.Errorf("bvc token chain query returned error, %v", err)
	}
	qResp = &aResp.Response

	var resp *acmeApi.APIDataResponse
	if len(txId) == 0 {
		chainHeader := state.Chain{}

		err = chainHeader.UnmarshalBinary(qResp.Value)

		if err != nil {
			return nil, fmt.Errorf("invalid state object returned from query of url %s, %v", *adiChainPath, err)
		}

		//unmarshal the state object if available
		if val, ok := ChainStates[chainHeader.Type]; ok {
			//resp, err = val.(func([]byte) (interface{}, error))(qResp.Value)
			resp, err = val.(func(*Query, *string, []byte) (*acmeApi.APIDataResponse, error))(q, chainHeader.ChainUrl.AsString(), []byte{})
		} else {
			resp = &acmeApi.APIDataResponse{}
			resp.Type = types.String(chainHeader.Type.Name())
			msg := json.RawMessage{}
			msg = []byte(fmt.Sprintf("{\"entry\":\"%x\"}", qResp.Value))
			resp.Data = &msg
		}
	}

	return resp, err
}
