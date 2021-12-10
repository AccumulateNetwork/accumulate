package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/relay"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api"
	acmeApi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
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
	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	qu := query.Query{}
	qu.RouteId = u.Routing()
	qu.Type = types.QueryTypeUrl
	ru := query.RequestByUrl{}
	ru.Url = types.String(u.String())
	qu.Content, err = ru.MarshalBinary()
	if err != nil {
		return nil, err
	}

	qd, err := qu.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return q.txRelay.Query(qu.RouteId, qd)
}

func (q *Query) QueryDirectoryByUrl(url string) (*ctypes.ResultABCIQuery, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	qu := query.Query{}
	qu.RouteId = u.Routing()
	qu.Type = types.QueryTypeDirectoryUrl
	ru := query.RequestDirectory{}
	ru.Url = types.String(u.String())
	qu.Content, err = ru.MarshalBinary()
	if err != nil {
		return nil, err
	}

	qd, err := qu.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return q.txRelay.Query(qu.RouteId, qd)
}

func (q *Query) QueryByTxId(txId []byte) (resp *ctypes.ResultABCIQuery, err error) {
	qu := query.Query{}
	qu.Type = types.QueryTypeTxId
	txq := query.RequestByTxId{}
	txq.TxId.FromBytes(txId)
	qu.Content, err = txq.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return q.queryAll(&qu)
}

func (q *Query) QueryByChainId(chainId []byte) (ret *ctypes.ResultABCIQuery, err error) {
	qu := query.Query{}
	qc := query.RequestByChainId{}
	qc.ChainId.FromBytes(chainId)
	qu.Content, err = qc.MarshalBinary()
	if err != nil {
		return nil, err
	}
	ret, err = q.queryAll(&qu)
	return ret, err
}

// queryAll
// Will search all networks for the information.  Once found, it will return the results.
func (q *Query) queryAll(apiQuery *query.Query) (ret *ctypes.ResultABCIQuery, err error) {
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
		switch {
		case res.err != nil:
			err = res.err
		case res.ret == nil:
			err = errors.New("invalid response")
		case res.ret.Response.Code == 0:
			ret = res.ret
		default:
			err = errors.New(res.ret.Response.Info)
		}
	}

	if ret != nil {
		return ret, nil
	}
	return nil, err
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

	return unmarshalQueryResponse(r.Response, types.ChainTypeIdentity)
}

// GetToken
// retrieve the informatin regarding a token
func (q *Query) GetToken(tokenUrl string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(tokenUrl)
	if err != nil {
		return nil, fmt.Errorf("bvc token query returned error, %v", err)
	}

	return unmarshalQueryResponse(r.Response, types.ChainTypeTokenIssuer)
}

// GetTokenAccount get the token balance for a given url
func (q *Query) GetTokenAccount(adiChainPath string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(adiChainPath)
	if err != nil {
		return nil, fmt.Errorf("bvc token account query returned error, %v", err)
	}

	return unmarshalQueryResponse(r.Response, types.ChainTypeTokenAccount, types.ChainTypeLiteTokenAccount)
}

// GetDirectory returns directory entries for a given url
func (q *Query) GetDirectory(url string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryDirectoryByUrl(url)
	if err != nil {
		return nil, fmt.Errorf("bvc directory query returned error, %v", err)
	}
	if err := responseIsError(r.Response); err != nil {
		return nil, err
	}

	dir := new(protocol.DirectoryQueryResult)
	err = dir.UnmarshalBinary(r.Response.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}

	data, err := json.Marshal(dir)
	if err != nil {
		return nil, err
	}

	rAPI := new(api.APIDataResponse)
	rAPI.Type = "directory"
	rAPI.Data = (*json.RawMessage)(&data)
	return rAPI, nil
}

// GetTransactionReference get the transaction id for a given transaction number
func (q *Query) GetTransactionReference(adiChainPath string) (*acmeApi.APIDataResponse, error) {
	r, err := q.QueryByUrl(adiChainPath)
	if err != nil {
		return nil, fmt.Errorf("transaction id reference chain query returned error, %v", err)
	}

	return unmarshalTxReference(r.Response)
}

// QueryDataSetByUrl returns the data specified by the pagination information on given chain specified by the url
func (q *Query) QueryDataSetByUrl(url string, start uint64, limit uint64) (*acmeApi.APIDataResponsePagination, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	qu := query.Query{}
	qu.RouteId = u.Routing()
	qu.Type = types.QueryTypeDataSet
	ru := protocol.RequestDataEntrySet{} //change to RequestDataSet{}
	ru.Start = start
	ru.Limit = limit
	ru.Url = u.String()
	qu.Content, err = ru.MarshalBinary()
	if err != nil {
		return nil, err
	}

	qd, err := qu.MarshalBinary()
	if err != nil {
		return nil, err
	}

	res, err := q.txRelay.Query(qu.RouteId, qd)
	if err != nil {
		return nil, err
	}

	if res.Response.Code != 0 {
		return nil, fmt.Errorf("query failed with code %d: %q", res.Response.Code, res.Response.Info)
	}
	thr := protocol.ResponseDataEntrySet{}
	err = thr.UnmarshalBinary(res.Response.Value)
	if err != nil {
		return nil, err
	}

	ret := acmeApi.APIDataResponsePagination{}
	ret.Start = int64(start)
	ret.Limit = int64(limit)
	ret.Total = int64(thr.Total)
	for i := range thr.DataEntries {
		entry := thr.DataEntries[i]
		dr := acmeApi.APIDataResponse{}
		d, err := json.Marshal(&entry)
		if err != nil {
			return nil, err
		}
		dr.Data = &json.RawMessage{}
		*dr.Data = d
		dr.Sponsor = types.String(u.String())
		dr.Type = "dataEntry"

		//todo: the following are unknown, but can be derived via another query, is that appropriate?
		//dr.TxId = ??
		//dr.Sig = ??
		//dr.MerkleState = ??
		//dr.KeyPage??
		//dr.Status = ??

		ret.Data = append(ret.Data, &dr)
	}

	return &ret, nil
}

// QueryDataByEntryHash returns the data specified by the entry hash in a given chain specified by the url
func (q *Query) QueryDataByEntryHash(url string, entryHash []byte) (*acmeApi.APIDataResponse, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	qu := query.Query{}
	qu.RouteId = u.Routing()
	qu.Type = types.QueryTypeDataEntry
	ru := protocol.RequestDataEntryHash{}
	ru.Url = u.String()
	copy(ru.EntryHash[:], entryHash)

	qu.Content, err = ru.MarshalBinary()
	if err != nil {
		return nil, err
	}

	qd, err := qu.MarshalBinary()
	if err != nil {
		return nil, err
	}

	res, err := q.txRelay.Query(qu.RouteId, qd)
	if err != nil {
		return nil, err
	}

	if res.Response.Code != 0 {
		return nil, fmt.Errorf("query failed with code %d: %q", res.Response.Code, res.Response.Info)
	}
	thr := protocol.ResponseDataEntry{}
	err = thr.UnmarshalBinary(res.Response.Value)
	if err != nil {
		return nil, err
	}

	dr := acmeApi.APIDataResponse{}
	d, err := thr.MarshalJSON()
	if err != nil {
		return nil, err
	}

	dr.Data = &json.RawMessage{}
	*dr.Data = d
	dr.Sponsor = types.String(u.String())
	dr.Type = "dataEntry"

	//todo: the following are unknown, but can be derived via another query, is that appropriate?
	//dr.TxId = ??
	//dr.Sig = ??
	//dr.MerkleState = ??
	//dr.KeyPage??
	//dr.Status = ??

	return &dr, nil
}

// QueryDataByUrl returns the current data at the head of the data chain
func (q *Query) QueryDataByUrl(url string) (*acmeApi.APIDataResponse, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	qu := query.Query{}
	qu.RouteId = u.Routing()
	qu.Type = types.QueryTypeDataUrl
	ru := protocol.RequestDataEntry{}
	ru.Url = u.String()

	qu.Content, err = ru.MarshalBinary()
	if err != nil {
		return nil, err
	}

	qd, err := qu.MarshalBinary()
	if err != nil {
		return nil, err
	}

	res, err := q.txRelay.Query(qu.RouteId, qd)
	if err != nil {
		return nil, err
	}

	if res.Response.Code != 0 {
		return nil, fmt.Errorf("query failed with code %d: %q", res.Response.Code, res.Response.Info)
	}
	thr := protocol.ResponseDataEntry{}
	err = thr.UnmarshalBinary(res.Response.Value)
	if err != nil {
		return nil, err
	}

	dr := acmeApi.APIDataResponse{}
	d, err := thr.MarshalJSON()
	if err != nil {
		return nil, err
	}

	dr.Data = &json.RawMessage{}
	*dr.Data = d
	dr.Sponsor = types.String(u.String())
	dr.Type = "dataEntry"

	//todo: the following are unknown, but can be derived via another query, is that appropriate?
	//dr.TxId = ??
	//dr.Sig = ??
	//dr.MerkleState = ??
	//dr.KeyPage??
	//dr.Status = ??

	return &dr, nil
}

// packTransactionQuery
func packTransactionQuery(txId []byte, txData []byte, txPendingData []byte, txSynthTxIds []byte) (resp *acmeApi.APIDataResponse, err error) {

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
	if txErr == nil {
		txState := state.Transaction{}
		err = txState.UnmarshalBinary(txObject.Entry)
		if err != nil {
			return resp, accumulateError(err)
		}
		txStateData = txState.Transaction
		txSigInfo = txState.SigInfo
	}

	var txPendingState *state.PendingTransaction
	if pendErr == nil {
		txPendingState = &state.PendingTransaction{}
		pendErr = txPendingState.UnmarshalBinary(txPendingObject.Entry)
		if pendErr != nil {
			return nil, accumulateError(fmt.Errorf("invalid pending object entry %v", pendErr))
		}

		if txStateData == nil {
			if txPendingState.TransactionState == nil {
				return nil, accumulateError(fmt.Errorf("no transaction state for transaction on pending or main chains"))
			}
			txStateData = txPendingState.TransactionState.Transaction
			txSigInfo = txPendingState.TransactionState.SigInfo
		}
	}

	resp, err = unmarshalTransaction(txSigInfo, txStateData.Bytes(), txId, txSynthTxIds)
	if err != nil {
		return nil, accumulateError(err)
	}

	//populate the rest of the resp
	resp.TxId = (*types.Bytes)(&txId)
	resp.KeyPage = &acmeApi.APIRequestKeyPage{}
	resp.KeyPage.Height = txSigInfo.KeyPageHeight
	resp.KeyPage.Index = txSigInfo.KeyPageIndex

	//if we have pending data (i.e. signature stuff, populate that too.)
	if txPendingState != nil && len(txPendingState.Signature) > 0 {
		//if the pending state still exists
		resp.Status = &txPendingState.Status
		resp.Signer = &acmeApi.Signer{}
		resp.Signer.PublicKey.FromBytes(txPendingState.Signature[0].PublicKey)
		if len(txPendingState.Signature) == 0 {
			return nil, accumulateError(fmt.Errorf("malformed transaction, no signatures"))
		}
		resp.Signer.Nonce = txPendingState.Signature[0].Nonce
		sig := types.Bytes(txPendingState.Signature[0].Signature).AsBytes64()
		resp.Sig = &sig
	}
	return resp, err
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

	rid := query.ResponseByTxId{}
	err = rid.UnmarshalBinary(qResp.Value)
	txData := rid.TxState
	txPendingData := rid.TxPendingState
	txSynthTxIds := rid.TxSynthTxIds
	return packTransactionQuery(txId, txData, txPendingData, txSynthTxIds)
}

func (q *Query) GetTransactionHistory(url string, start int64, limit int64) (*api.APIDataResponsePagination, error) {

	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	qu := query.Query{}
	qu.RouteId = u.Routing()
	qu.Type = types.QueryTypeTxHistory
	ru := query.RequestTxHistory{}
	ru.Start = start
	ru.Limit = limit
	ru.ChainId.FromBytes(u.ResourceChain())
	qu.Content, err = ru.MarshalBinary()
	if err != nil {
		return nil, err
	}

	qd, err := qu.MarshalBinary()
	if err != nil {
		return nil, err
	}

	res, err := q.txRelay.Query(qu.RouteId, qd)
	if err != nil {
		return nil, err
	}
	if res.Response.Code != 0 {
		return nil, fmt.Errorf("query failed with code %d: %q", res.Response.Code, res.Response.Info)
	}
	thr := query.ResponseTxHistory{}
	err = thr.UnmarshalBinary(res.Response.Value)
	if err != nil {
		return nil, err
	}
	//res.Response.Value
	ret := acmeApi.APIDataResponsePagination{}
	ret.Start = start
	ret.Limit = limit
	ret.Total = thr.Total
	for i := range thr.Transactions {
		txs := thr.Transactions[i]

		txData := txs.TxState
		txPendingData := txs.TxPendingState
		txSynthTxIds := txs.TxSynthTxIds
		d, err := packTransactionQuery(txs.TxId[:], txData, txPendingData, txSynthTxIds)
		if err != nil {
			return nil, err
		}
		ret.Data = append(ret.Data, d)
	}

	return &ret, nil
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

	return unmarshalQueryResponse(r.Response)
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

	return unmarshalQueryResponse(r.Response)
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

	return unmarshalQueryResponse(r.Response)
}
