package router

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	proto1 "github.com/golang/protobuf/proto"
	"github.com/tendermint/tendermint/abci/client"
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

func (q *Query) GetAdi(adi *string) (interface{}, error) {

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
	resp, err := q.client.QuerySync(req)

	if err != nil {
		return nil, fmt.Errorf("bvc adi query returned error, %v", err)
	}

	//unpack the response
	adiResp := response.ADI{}
	err = adiResp.ADI.UnmarshalBinary(resp.Value)

	if err != nil {
		return nil, fmt.Errorf("cannot extract adi information")
	}

	return adiResp, nil
}

func (q *Query) GetToken(tokenUrl *string) (interface{}, error) {

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
	tokResp := response.Token{}
	err = tokResp.Token.UnmarshalBinary(qResp.Value)

	if err != nil {
		return nil, fmt.Errorf("cannot extract token information")
	}

	return tokResp, err
}

func (q *Query) GetTokenAccount(adiChainPath *string) (interface{}, error) {

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
	//qResp, err := q.client.QuerySync(req)
	//
	//if err != nil {
	//	return nil, fmt.Errorf("bvc token query returned error, %v", err)
	//}

	//unpack the response
	tokResp := response.TokenAccount{}
	//err = tokResp.UnmarshalBinary(qResp.Value)

	if err != nil {
		return nil, fmt.Errorf("cannot extract token information")
	}

	return tokResp, err
}

func (q *Query) GetTokenTx(tokenAccountUrl *string, txid []byte) (resp interface{}, err error) {
	// need to know the ADI and ChainID, deriving adi and chain id from TokenTx.From
	pq := proto.Query{}
	pq.ChainUrl = *tokenAccountUrl
	adichain := types.GetIdentityChainFromIdentity(&pq.ChainUrl)
	chainId := types.GetChainIdFromChainPath(&pq.ChainUrl)
	pq.AdiChain = adichain.Bytes()
	pq.ChainId = chainId.Bytes()
	pq.Ins = proto.AccInstruction_Token_Transaction
	pq.Query = txid

	req := tmtypes.RequestQuery{}

	req.Data, err = proto1.Marshal(&pq)

	if err != nil {
		return resp, fmt.Errorf("cannot marshal query")
	}

	//this is only temporary until we get router setup.
	qResp, err := q.client.QuerySync(req)

	tx := acmeapi.TokenTx{}
	err = json.Unmarshal(qResp.Value, &tx)

	if err != nil {
		return resp, NewAccumulateError(err)
	}
	txResp := response.TokenTx{}
	txResp.FromUrl = types.String(*tokenAccountUrl)
	copy(txResp.TxId[:], txid)

	//should receive tx,unmarshal to To accounts
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
	*acmeapi.ChainTypeAdi.AsBytes32(): func(data []byte) (interface{}, error) {
		r := state.AdiState{}
		if err := r.UnmarshalBinary(data); err != nil {
			return nil, err
		}
		return &r, nil
	},
	*acmeapi.ChainTypeToken.AsBytes32(): func(data []byte) (interface{}, error) {
		r := state.Token{}
		if err := r.UnmarshalBinary(data); err != nil {
			return nil, err
		}
		return &r, nil
	},
	*acmeapi.ChainTypeTokenAccount.AsBytes32(): func(data []byte) (interface{}, error) {
		r := state.TokenAccount{}
		if err := r.UnmarshalBinary(data); err != nil {
			return nil, err
		}
		return &r, nil
	},
	*acmeapi.ChainTypeAnonTokenAccount.AsBytes32(): func(data []byte) (interface{}, error) {
		r := state.Chain{}
		if err := r.UnmarshalBinary(data); err != nil {
			return nil, err
		}
		return &r, nil
	},
}

// getChainState will return the state object of the chain, which include the chain
// header and the current state data for the chain
func (q *Query) getChainState(adiChainPath *string) (interface{}, error) {

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

	var resp interface{}

	//unmarshal the state object if available
	if val, ok := ChainStates[chainHeader.Type]; ok {
		resp, err = val.(func([]byte) (interface{}, error))(qResp.Value)
	} else {
		err = fmt.Errorf("unable to unmarshal state object for chain of type %s at %s", acmeapi.ChainTypeSpecMap[chainHeader.Type], chainHeader.ChainUrl)
	}

	return resp, err
}
