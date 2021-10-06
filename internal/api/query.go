package api

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	acmeApi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
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
	r, err := q.Query(*adi, nil)
	if err != nil {
		return nil, fmt.Errorf("bvc adi query returned error, %v", err)
	}

	return unmarshalADI(r.Response)
}

// GetToken
// retrieve the informatin regarding a token
func (q *Query) GetToken(tokenUrl *string) (*acmeApi.APIDataResponse, error) {
	r, err := q.Query(*tokenUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("bvc token query returned error, %v", err)
	}

	return unmarshalToken(r.Response)
}

// GetTokenAccount get the token balance for a given url
func (q *Query) GetTokenAccount(adiChainPath *string) (*acmeApi.APIDataResponse, error) {
	r, err := q.Query(*adiChainPath, nil)
	if err != nil {
		return nil, fmt.Errorf("bvc token account query returned error, %v", err)
	}

	return unmarshalTokenAccount(r.Response)
}

// GetTokenTx

// GetTransaction
// get the tx from the primary url, if the transaction spawned synthetic tx's then return the synth txid's
func (q *Query) GetTransaction(url string, txId []byte) (resp *acmeApi.APIDataResponse, err error) {

	aResp, err := q.Query(url, txId)
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

	return unmarshalTransaction(url, txState.Transaction.Bytes(), txId, txSynthTxIds)
}

// GetChainState
// will return the state object of the chain, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainState(adiChainPath *string, txId []byte) (*api.APIDataResponse, error) {
	//this QuerySync call is only temporary until we get router setup.
	r, err := q.Query(*adiChainPath, txId)
	if err != nil {
		return nil, fmt.Errorf("chain query returned error, %v", err)
	}

	return q.unmarshalChainState(r.Response)
}
