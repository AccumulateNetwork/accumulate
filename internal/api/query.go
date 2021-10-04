package api

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	acmeApi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
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
// get the token tx from the primary adi, then query all the output accounts to get the status
func (q *Query) GetTokenTx(tokenAccountUrl *string, txId []byte) (resp interface{}, err error) {
	// need to know the ADI and ChainID, deriving adi and chain id from TokenTx.From

	// TODO Why does this return a response.TokenTx instead of api.APIDataResponse?

	r, err := q.Query(*tokenAccountUrl, txId)
	if err != nil {
		return nil, fmt.Errorf("bvc token tx query returned error, %v", err)
	}

	tx, err := unmarshalTokenTx(r.Response)
	if err != nil {
		return nil, err
	}

	rTx := new(response.TokenTx)
	rTx.FromUrl = types.String(*tokenAccountUrl)
	copy(rTx.TxId[:], txId)

	//should receive tx,unmarshal to output accounts
	for _, v := range tx.Outputs {
		// TODO What is the purpose of this? It's executing the same query, repeatedly.
		aResp, err := q.Query(*tokenAccountUrl, txId)

		txStatus := response.TokenTxAccountStatus{}
		if err != nil {
			txStatus.Status = types.String(fmt.Sprintf("transaction not found for %s, %v", v.Dest, err))
			txStatus.AccountUrl = types.String(v.Dest)
		} else {
			qResp := aResp.Response
			err = txStatus.UnmarshalBinary(qResp.Value)
			if err != nil {
				txStatus.Status = types.String(fmt.Sprintf("%v", err))
				txStatus.AccountUrl = types.String(v.Dest)
			}
		}

		rTx.ToAccount = append(rTx.ToAccount, txStatus)
	}

	return rTx, err
}

// GetChainState
// will return the state object of the chain, which include the chain
// header and the current state data for the chain
func (q *Query) GetChainState(adiChainPath *string, txId []byte) (*api.APIDataResponse, error) {
	//this QuerySync call is only temporary until we get router setup.
	r, err := q.Query(*adiChainPath, txId)
	if err != nil {
		return nil, fmt.Errorf("bvc token chain query returned error, %v", err)
	}

	return q.unmarshalByType(r.Response)
}
