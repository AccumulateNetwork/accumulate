package api

import (
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	acmeApi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

type Query struct {
	txRelay *relay.Relay
}

type queryData struct {
	ret *ctypes.ResultABCIQuery
	err error
}

func NewQuery(txRelay *relay.Relay) *Query {
	q := Query{}
	q.txRelay = txRelay
	return &q
}

func (q *Query) BroadcastTx(gtx *transactions.GenTransaction, done chan abci.TxResult) (ti relay.TransactionInfo, err error) {
	payload, err := gtx.Marshal()
	if err != nil {
		return ti, err
	}

	if done != nil {
		err = q.txRelay.SubscribeTx(sha256.Sum256(payload), done)
		if err != nil {
			return ti, err
		}
	}

	ti = q.txRelay.BatchTx(gtx.Routing, payload)
	return ti, nil
}

func (q *Query) SubscribeTx(txRef [32]byte, done chan abci.TxResult) error {
	return q.txRelay.SubscribeTx(txRef, done)
}

func (q *Query) GetTx(routing uint64, txRef [32]byte) (*ctypes.ResultTx, error) {
	return q.txRelay.GetTx(routing, txRef[:])
}

func (q *Query) QueryByUrl(url string) (*ctypes.ResultABCIQuery, error) {
	addr := types.GetAddressFromIdentity(&url)

	query := api.Query{}
	query.Url = url
	query.RouteId = addr
	query.ChainId = types.GetChainIdFromChainPath(&url).Bytes()

	payload, err := query.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return q.txRelay.Query(addr, payload)
}

func (q *Query) QueryByTxId(txId []byte) (*ctypes.ResultABCIQuery, error) {
	query := api.Query{}
	query.Content = txId
	return q.queryAll(&query)
}

func (q *Query) QueryByChainId(chainId []byte) (ret *ctypes.ResultABCIQuery, err error) {
	query := api.Query{}
	query.ChainId = chainId
	ret, err = q.queryAll(&query)
	return ret, err
}

// queryAll
// Will search all networks for the information.  Once found, it will return the results.
func (q *Query) queryAll(apiQuery *api.Query) (ret *ctypes.ResultABCIQuery, err error) {
	//TODO: when the data servers become a thing, we will query that instead to get the information we need
	//in the mean time, we will need to ping all the bvc's for the needed info.  not ideal by any means, but it works.
	var results []chan queryData
	for i := uint64(0); i < q.txRelay.GetNetworkCount(); i++ {
		results = append(results, make(chan queryData))
		apiQuery.RouteId = i
		payload, err := apiQuery.MarshalBinary()
		if err != nil {
			return nil, err
		}
		go q.query(i, payload, results[i])
	}

	for i := range results {
		res := <-results[i]
		if res.err == nil && res.ret != nil {
			if res.ret.Response.Code == 0 {
				ret = res.ret
				err = res.err
				//we found a match
				break
			}
		}
	}

	return ret, err
}

//query
//internal query call to a targeted bvc.  populates the channel with the query results
func (q *Query) query(i uint64, payload bytes.HexBytes, r chan queryData) {

	qd := queryData{}
	qd.ret, qd.err = q.txRelay.Query(i, payload)

	r <- qd
}

// BatchSend calls the underlying client's BatchSend method, if it has one
func (q *Query) BatchSend() <-chan relay.BatchedStatus {
	return q.txRelay.BatchSend()
}

//"GetADI()"
//"GetToken()"
//"GetTokenAccount()"
//"GetTokenTx()"
//"GetData()" Submit url into, and receive ADI/Token/TokenAccount/TokenTx

// GetAdi get the adi state object. Use this to get the nonce.
func (q *Query) GetAdi(adi string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(adi)
	if err != nil {
		return nil, fmt.Errorf("bvc adi query returned error, %v", err)
	}

	return unmarshalADI(r.Response)
}

// GetToken
// retrieve the informatin regarding a token
func (q *Query) GetToken(tokenUrl string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(tokenUrl)
	if err != nil {
		return nil, fmt.Errorf("bvc token query returned error, %v", err)
	}

	return unmarshalToken(r.Response)
}

// GetTokenAccount get the token balance for a given url
func (q *Query) GetTokenAccount(adiChainPath string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(adiChainPath)
	if err != nil {
		return nil, fmt.Errorf("bvc token account query returned error, %v", err)
	}

	return unmarshalChainState(r.Response)
}

// GetTransactionReference get the transaction id for a given transaction number
func (q *Query) GetTransactionReference(adiChainPath string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(adiChainPath)
	if err != nil {
		return nil, fmt.Errorf("transaction id reference chain query returned error, %v", err)
	}

	return unmarshalTxReference(r.Response)
}

// GetTokenTx

// GetTransaction
// get the tx from the primary url, if the transaction spawned synthetic tx's then return the synth txid's
func (q *Query) GetTransaction(txId []byte) (resp *acmeApi.APIDataResponse, err error) {

	aResp, err := q.QueryByTxId(txId)
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

	if txSynthTxIdsRaw == nil {
		return nil, fmt.Errorf("unable to obtain synth txids")
	}

	txSynthTxIds, _ := common.BytesSlice(txSynthTxIdsRaw)

	if len(txSynthTxIds)%32 != 0 {
		return nil, fmt.Errorf("invalid synth txids")
	}

	txObject := state.Object{}
	txPendingObject := state.Object{}
	pendErr := txPendingObject.UnmarshalBinary(txPendingData)
	//not having pending is ok since pending status can be purged.  This is only an error if there
	//is no transaction object either
	txErr := txObject.UnmarshalBinary(txData)
	if txErr != nil && pendErr != nil {
		return nil, fmt.Errorf("invalid transaction object for query, %v", txErr)
	}

	var txStateData *types.Bytes
	var txSigInfo *transactions.SignatureInfo
	var sponsor types.String
	if txErr == nil {
		txState := state.Transaction{}
		err = txState.UnmarshalBinary(txObject.Entry)
		if err != nil {
			return resp, NewAccumulateError(err)
		}
		txStateData = txState.Transaction
		txSigInfo = txState.SigInfo
		sponsor = txState.ChainUrl
	}

	var txPendingState *state.PendingTransaction
	if pendErr == nil {
		txPendingState = &state.PendingTransaction{}
		pendErr = txPendingState.UnmarshalBinary(txPendingObject.Entry)
		if pendErr != nil {
			return nil, NewAccumulateError(fmt.Errorf("invalid pending object entry %v", pendErr))
		}

		if txStateData == nil {
			if txPendingState.TransactionState == nil {
				return nil, NewAccumulateError(fmt.Errorf("no transaction state for transaction on pending or main chains"))
			}
			txStateData = txPendingState.TransactionState.Transaction
			txSigInfo = txPendingState.TransactionState.SigInfo
			sponsor = txPendingState.ChainUrl
		}
	}

	resp, err = unmarshalTransaction(txStateData.Bytes(), txId, txSynthTxIds)
	if err != nil {
		return nil, NewAccumulateError(err)
	}

	//populate the rest of the resp
	resp.Sponsor = sponsor
	resp.KeyPage = &acmeApi.APIRequestKeyPage{}
	resp.KeyPage.Height = txSigInfo.MSHeight
	resp.KeyPage.Index = txSigInfo.PriorityIdx

	//if we have pending data (i.e. signature stuff, populate that too.)
	if txPendingState != nil {
		//if the pending state still exists
		resp.Status = &txPendingState.Status
		resp.Signer = &acmeApi.Signer{}
		resp.Signer.PublicKey.FromBytes(txPendingState.Signature[0].PublicKey)
		if len(txPendingState.Signature) == 0 {
			return nil, NewAccumulateError(fmt.Errorf("malformed transaction, no signatures"))
		}
		resp.Signer.Nonce = txPendingState.Signature[0].Nonce
		resp.Sig.FromBytes(txPendingState.Signature[0].Signature)
	}
	return resp, err
}

// GetChainStateByUrl
// will return the state object of the chain given a Url, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainStateByUrl(adiChainPath string) (*api.APIDataResponse, error) {
	//this QuerySync call is only temporary until we get router setup.
	r, err := q.QueryByUrl(adiChainPath)
	if err != nil {
		return nil, fmt.Errorf("chain query returned error, %v", err)
	}

	return unmarshalChainState(r.Response)
}

// GetChainStateByTxId
// will return the state of a transaction given the TxId, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainStateByTxId(txId []byte) (*api.APIDataResponse, error) {
	//this QuerySync call is only temporary until we get router setup.
	r, err := q.QueryByTxId(txId)
	if err != nil {
		return nil, fmt.Errorf("chain query returned error, %v", err)
	}

	return unmarshalChainState(r.Response)
}

// GetChainStateByChainId
// will return the state object of the chain given a chainId, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainStateByChainId(chainId []byte) (*api.APIDataResponse, error) {
	//this QuerySync call is only temporary until we get router setup.
	r, err := q.QueryByChainId(chainId)
	if err != nil {
		return nil, fmt.Errorf("chain query returned error, %v", err)
	}

	return unmarshalChainState(r.Response)
}
