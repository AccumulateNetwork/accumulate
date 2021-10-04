package api

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	acmeApi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/state"
	tmtypes "github.com/tendermint/tendermint/abci/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
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

func (q *Query) BroadcastTx(gtx *transactions.GenTransaction) (*ctypes.ResultBroadcastTx, error) {
	payload, err := gtx.Marshal()
	if err != nil {
		return nil, err
	}
	return q.txBouncer.BatchTx(payload)
}

func (q *Query) Query(url string, txid []byte) (*ctypes.ResultABCIQuery, error) {
	addr := types.GetAddressFromIdentity(&url)

	query := api.Query{}
	query.Url = url
	query.RouteId = addr
	query.ChainId = types.GetChainIdFromChainPath(&url).Bytes()
	query.Content = txid

	payload, err := query.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return q.txBouncer.Query(payload)
}

// BatchSend calls the underlying client's BatchSend method, if it has one
func (q *Query) BatchSend() {
	q.txBouncer.BatchSend()
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

	aResp, err := q.Query(*adi, nil)

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
	aResp, err := q.Query(*tokenUrl, nil)

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

	aResp, err := q.Query(*adiChainPath, nil)

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

func (q *Query) packTokenTxResponse(tokenTxRaw []byte, txId types.Bytes, txSynthTxIds types.Bytes) (*acmeApi.APIDataResponse, error) {
	tx := acmeApi.TokenTx{}
	err := tx.UnmarshalBinary(tokenTxRaw)
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
	resp := acmeApi.APIDataResponse{}
	resp.Type = types.String(types.TxTypeTokenTx.Name())
	resp.Data = new(json.RawMessage)
	*resp.Data = data
	return &resp, err
}

func (q *Query) packSynthTokenDepositResponse(tokenTxRaw []byte, txId types.Bytes, txSynthTxIds types.Bytes) (*acmeApi.APIDataResponse, error) {
	tx := synthetic.TokenTransactionDeposit{}
	err := tx.UnmarshalBinary(tokenTxRaw)
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
	resp := acmeApi.APIDataResponse{}
	resp.Type = types.String(types.TxTypeSyntheticTokenDeposit.Name())
	resp.Data = new(json.RawMessage)
	*resp.Data = data
	return &resp, err
}

// GetTokenTx
// get the token tx from the primary adi, then query all the output accounts to get the status
func (q *Query) GetTokenTx(tokenAccountUrl *string, txId []byte) (resp interface{}, err error) {
	// need to know the ADI and ChainID, deriving adi and chain id from TokenTx.From

	aResp, err := q.Query(*tokenAccountUrl, txId)

	if err != nil {
		return nil, fmt.Errorf("bvc token tx query returned error, %v", err)
	}
	qResp := aResp.Response

	if qResp.Value == nil {
		return nil, fmt.Errorf("no data available for txid %x", txId)
	}

	txData, txPendingRaw := common.BytesSlice(qResp.Value)
	if txPendingRaw == nil {
		return nil, fmt.Errorf("unable to obtain data from value")
	}
	txPendingData, txSynthTxIdsRaw := common.BytesSlice(txPendingRaw)
	_ = txPendingData

	if txSynthTxIdsRaw == nil {
		return nil, fmt.Errorf("unable to obtain synth txids")
	}

	txSynthTxIds, _ := common.BytesSlice(txSynthTxIdsRaw)

	if len(txSynthTxIds)%32 != 0 {
		return nil, fmt.Errorf("invalid synth txids")
	}

	txObject := state.Object{}
	err = txObject.UnmarshalBinary(txData)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction object for query, %v", err)
	}

	txState := state.Transaction{}
	err = txState.UnmarshalBinary(txObject.Entry)
	if err != nil {
		return resp, NewAccumulateError(err)
	}

	txType, _ := common.BytesUint64(txState.Transaction.Bytes())
	switch types.TxType(txType) {
	case types.TxTypeTokenTx:
		resp, err = q.packTokenTxResponse(txState.Transaction.Bytes(), txId, txSynthTxIds)
	case types.TxTypeSyntheticTokenDeposit:
		resp, err = q.packSynthTokenDepositResponse(txState.Transaction.Bytes(), txId, txSynthTxIds)
	default:
		err = fmt.Errorf("unable to extract transaction info for type %s : %x", types.TxType(txType).Name(), txState.Transaction.Bytes())
	}

	return resp, err
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
	aResp, err := q.Query(*adiChainPath, txId)
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
