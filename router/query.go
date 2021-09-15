package router

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	acmeApi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	proto1 "github.com/golang/protobuf/proto"
	abcicli "github.com/tendermint/tendermint/abci/client"
	tmtypes "github.com/tendermint/tendermint/abci/types"
)

type Query struct {
	client abcicli.Client
}

func NewQuery(app tmtypes.Application) *Query {
	q := Query{}
	q.client = abcicli.NewLocalClient(nil, app)
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

	pq := proto.Query{}
	pq.ChainUrl = *adi
	adiChain := types.GetIdentityChainFromIdentity(adi)
	pq.AdiChain = adiChain.Bytes()
	pq.Ins = proto.AccInstruction_Token_Issue

	req := tmtypes.RequestQuery{}

	req.Data, err = proto1.Marshal(&pq)

	if err != nil {
		return nil, fmt.Errorf("cannot marshal query for Token issuance")
	}

	//this is only temporary until we get router setup.
	qResp, err := q.client.QuerySync(req)

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

	pq := proto.Query{}
	pq.ChainUrl = *tokenUrl
	adiChain := types.GetIdentityChainFromIdentity(tokenUrl)
	chainId := types.GetChainIdFromChainPath(tokenUrl)
	pq.AdiChain = adiChain.Bytes()
	pq.ChainId = chainId.Bytes()
	pq.Ins = proto.AccInstruction_Token_Issue

	req := tmtypes.RequestQuery{}

	req.Data, err = proto1.Marshal(&pq)

	if err != nil {
		return nil, fmt.Errorf("cannot marshal query for Token issuance")
	}

	//this is only temporary until we get router setup.
	qResp, err := q.client.QuerySync(req)

	if err != nil {
		return nil, fmt.Errorf("bvc token query returned error, %v", err)
	}

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

	pq := proto.Query{}
	pq.ChainUrl = *adiChainPath
	adiChain := types.GetIdentityChainFromIdentity(adiChainPath)
	chainId := types.GetChainIdFromChainPath(adiChainPath)
	pq.AdiChain = adiChain.Bytes()
	pq.ChainId = chainId.Bytes()
	pq.Ins = proto.AccInstruction_Token_URL_Creation

	req := tmtypes.RequestQuery{}

	req.Data, err = proto1.Marshal(&pq)

	if err != nil {
		return nil, fmt.Errorf("cannot marshal query for Token issuance")
	}

	//this QuerySync call is only temporary until we get router setup.
	qResp, err := q.client.QuerySync(req)
	//
	if err != nil {
		return nil, fmt.Errorf("bvc token query returned error, %v", err)
	}

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
func (q *Query) GetTokenTx(tokenAccountUrl *string, txid []byte) (resp interface{}, err error) {
	// need to know the ADI and ChainID, deriving adi and chain id from TokenTx.From
	pq := proto.Query{}
	pq.ChainUrl = *tokenAccountUrl
	adiChain := types.GetIdentityChainFromIdentity(&pq.ChainUrl)
	//chainId := types.GetChainIdFromChainPath(&pq.ChainUrl)
	pq.AdiChain = adiChain.Bytes()
	pq.ChainId = txid //chainId.Bytes()
	pq.Ins = proto.AccInstruction_Token_Transaction
	//pq.Query = txid

	req := tmtypes.RequestQuery{}

	req.Data, err = proto1.Marshal(&pq)

	if err != nil {
		return resp, fmt.Errorf("cannot marshal query")
	}

	//this is only temporary until we get router setup.
	qResp, err := q.client.QuerySync(req)

	tx := acmeApi.TokenTx{}
	err = json.Unmarshal(qResp.Value, &tx)

	if err != nil {
		return resp, NewAccumulateError(err)
	}
	txResp := response.TokenTx{}
	txResp.FromUrl = types.String(*tokenAccountUrl)
	copy(txResp.TxId[:], txid)

	//should receive tx,unmarshal to output accounts
	for _, v := range tx.To {
		pq.ChainUrl = *v.URL.AsString()
		adiChain := types.GetIdentityChainFromIdentity(&pq.ChainUrl)
		chainId := types.GetChainIdFromChainPath(&pq.ChainUrl)
		pq.AdiChain = adiChain.Bytes()
		pq.ChainId = chainId.Bytes()
		pq.Ins = proto.AccInstruction_Token_Transaction
		pq.Query = txid

		req := tmtypes.RequestQuery{}

		req.Data, err = proto1.Marshal(&pq)

		if err != nil {
			return resp, fmt.Errorf("cannot marshal query")
		}

		//this is only temporary until we get a proper router setup.
		qResp, err := q.client.QuerySync(req)
		txStatus := response.TokenTxAccountStatus{}
		err = txStatus.UnmarshalBinary(qResp.Value)
		if err != nil {
			txStatus.Status = types.String(fmt.Sprintf("%v", err))
			txStatus.AccountUrl = v.URL.String
		}

		txResp.ToAccount = append(txResp.ToAccount, txStatus)
	}

	return txResp, err
}

var ChainStates = map[types.Bytes32]interface{}{
	*types.ChainTypeAdi.AsBytes32(): func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetAdi(url)
	},
	*types.ChainTypeToken.AsBytes32(): func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetToken(url)
	},
	*types.ChainTypeTokenAccount.AsBytes32(): func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		return q.GetTokenAccount(url)
	},
	*types.ChainTypeAnonTokenAccount.AsBytes32(): func(q *Query, url *string, txid []byte) (*acmeApi.APIDataResponse, error) {
		adi, _, _ := types.ParseIdentityChainPath(url)
		adi += "dc/ACME"
		return q.GetTokenAccount(url)
	},
}

// GetChainState
// will return the state object of the chain, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainState(adiChainPath *string) (interface{}, error) {
	var err error

	pq := proto.Query{}
	pq.ChainUrl = *adiChainPath
	adiChain := types.GetIdentityChainFromIdentity(adiChainPath)
	chainId := types.GetChainIdFromChainPath(adiChainPath)
	pq.AdiChain = adiChain.Bytes()
	pq.ChainId = chainId.Bytes()
	pq.Ins = proto.AccInstruction_State_Query

	req := tmtypes.RequestQuery{}

	req.Data, err = proto1.Marshal(&pq)

	if err != nil {
		return nil, fmt.Errorf("cannot marshal query for Token issuance")
	}

	//this QuerySync call is only temporary until we get router setup.
	qResp, err := q.client.QuerySync(req)

	chainHeader := state.Chain{}
	err = chainHeader.UnmarshalBinary(qResp.Value)

	if err != nil {
		return nil, fmt.Errorf("invalid state object returned from query, %v", err)
	}

	var resp *acmeApi.APIDataResponse

	//unmarshal the state object if available
	if val, ok := ChainStates[chainHeader.Type]; ok {
		//resp, err = val.(func([]byte) (interface{}, error))(qResp.Value)
		resp, err = val.(func(*Query, *string, []byte) (*acmeApi.APIDataResponse, error))(q, adiChainPath, []byte{})
	} else {
		err = fmt.Errorf("unable to unmarshal state object for chain of type %s at %s", types.ChainTypeSpecMap[chainHeader.Type], chainHeader.ChainUrl)
	}

	return resp, err
}
