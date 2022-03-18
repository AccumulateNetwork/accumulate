package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing/e2e"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func newKey(seed []byte) ed25519.PrivateKey {
	h := sha256.Sum256(seed)
	return ed25519.NewKeyFromSeed(h[:])
}

func makeLiteUrl(t *testing.T, key ed25519.PrivateKey, tok string) *url.URL {
	t.Helper()
	u, err := protocol.LiteTokenAddress(key[32:], tok)
	require.NoError(t, err)
	return u
}

func makeUrl(t *testing.T, s string) *url.URL {
	u, err := url.Parse(s)
	require.NoError(t, err)
	return u
}

func callApi(t *testing.T, japi *api.JrpcMethods, method string, params, result interface{}) interface{} {
	t.Helper()

	b, err := json.Marshal(params)
	require.NoError(t, err)

	r := japi.GetMethod(method)(context.Background(), b)
	err, _ = r.(error)
	require.NoError(t, err)

	if result == nil {
		return r
	}

	recode(t, r, result)
	return result
}

func queryRecord(t *testing.T, japi *api.JrpcMethods, method string, params interface{}) *api.ChainQueryResponse {
	t.Helper()

	r := new(api.ChainQueryResponse)
	callApi(t, japi, method, params, r)
	return r
}

func queryTxn(t *testing.T, japi *api.JrpcMethods, method string, params interface{}) *api.TransactionQueryResponse {
	t.Helper()

	r := new(api.TransactionQueryResponse)
	callApi(t, japi, method, params, r)
	return r
}

func queryRecordAs(t *testing.T, japi *api.JrpcMethods, method string, params, result interface{}) {
	t.Helper()
	r := queryRecord(t, japi, method, params)
	recode(t, r.Data, result)
}

type execParams struct {
	Origin  string
	Key     ed25519.PrivateKey
	Payload protocol.TransactionBody
}

func recode(t *testing.T, from, to interface{}) {
	t.Helper()

	b, err := json.Marshal(from)
	require.NoError(t, err)
	err = json.Unmarshal(b, to)
	require.NoError(t, err)
}

func prepareTx(t *testing.T, japi *api.JrpcMethods, params execParams) *api.TxRequest {
	t.Helper()
	u, err := url.Parse(params.Origin)
	require.NoError(t, err)

	var signator *url.URL
	if key, _, _ := protocol.ParseLiteTokenAddress(u); key != nil {
		signator = u
	} else {
		q := new(api.KeyPageIndexQuery)
		q.Url = u
		q.Key = params.Key.Public().(ed25519.PublicKey)
		qr := queryRecord(t, japi, "query-key-index", q)
		resp := new(query.ResponseKeyPageIndex)
		recode(t, qr.Data, resp)
		signator = resp.KeyPage
	}

	qr := queryRecord(t, japi, "query", &api.UrlQuery{Url: signator})
	env := acctesting.NewTransaction().
		WithPrincipal(u).
		WithSigner(signator, qr.MainChain.Height).
		WithNonceTimestamp().
		WithBody(params.Payload).
		Initiate(protocol.SignatureTypeLegacyED25519, params.Key)

	req := new(api.TxRequest)
	req.Origin = env.Transaction.Header.Principal
	req.Signer.Nonce = env.Signatures[0].GetTimestamp()
	req.Signer.Url = env.Signatures[0].GetSigner()
	req.Signer.PublicKey = env.Signatures[0].GetPublicKey()
	req.Signature = env.Signatures[0].GetSignature()
	req.KeyPage.Height = env.Signatures[0].GetSignerHeight()
	req.Payload = env.Transaction.Body
	return req
}

func executeTx(t *testing.T, japi *api.JrpcMethods, method string, wait bool, params execParams) *api.TxResponse {
	t.Helper()

	req := prepareTx(t, japi, params)
	resp := new(api.TxResponse)
	callApi(t, japi, method, req, resp)
	require.Zero(t, resp.Code, resp.Message)

	if wait {
		txWait(t, japi, resp.TransactionHash)
	}
	return resp
}

func executeTxFail(t *testing.T, japi *api.JrpcMethods, method string, keyPageUrl *url.URL, keyPageHeight uint64, params execParams) *api.TxResponse {
	t.Helper()

	u, err := url.Parse(params.Origin)
	require.NoError(t, err)

	env := acctesting.NewTransaction().
		WithPrincipal(u).
		WithSigner(keyPageUrl, keyPageHeight).
		WithNonceTimestamp().
		WithBody(params.Payload).
		Initiate(protocol.SignatureTypeLegacyED25519, params.Key)

	req := new(api.TxRequest)
	req.Origin = env.Transaction.Header.Principal
	req.Signer.Nonce = env.Signatures[0].GetTimestamp()
	req.Signer.Url = env.Signatures[0].GetSigner()
	req.Signer.PublicKey = env.Signatures[0].GetPublicKey()
	req.Signature = env.Signatures[0].GetSignature()
	req.KeyPage.Height = env.Signatures[0].GetSignerHeight()
	req.Payload = env.Transaction.Body

	resp := new(api.TxResponse)
	callApi(t, japi, method, req, resp)
	return resp
}

func txWait(t *testing.T, japi *api.JrpcMethods, txid []byte) {
	t.Helper()

	txr := queryTxn(t, japi, "query-tx", &api.TxnQuery{Txid: txid, Wait: 10 * time.Second})
	for _, txid := range txr.SyntheticTxids {
		queryTxn(t, japi, "query-tx", &api.TxnQuery{Txid: txid[:], Wait: 10 * time.Second})
	}
}

type e2eDUT struct {
	*e2e.Suite
	daemon *accumulated.Daemon
}

func (d *e2eDUT) api() *api.JrpcMethods {
	return d.daemon.Jrpc_TESTONLY()
}

func (d *e2eDUT) GetRecordAs(s string, target protocol.Account) {
	d.T().Helper()
	u, err := url.Parse(s)
	d.Require().NoError(err)
	r, err := d.api().Querier().QueryUrl(u, api.QueryOptions{})
	d.Require().NoError(err)
	d.Require().IsType((*api.ChainQueryResponse)(nil), r)
	qr := r.(*api.ChainQueryResponse)
	d.Require().IsType(target, qr.Data)
	reflect.ValueOf(target).Elem().Set(reflect.ValueOf(qr.Data).Elem())
}

func (d *e2eDUT) GetRecordHeight(s string) uint64 {
	d.T().Helper()
	u, err := url.Parse(s)
	d.Require().NoError(err)
	r, err := d.api().Querier().QueryUrl(u, api.QueryOptions{})
	d.Require().NoError(err)
	d.Require().IsType((*api.ChainQueryResponse)(nil), r)
	qr := r.(*api.ChainQueryResponse)
	return qr.MainChain.Height
}

func (d *e2eDUT) SubmitTxn(tx *protocol.Envelope) {
	data, err := tx.Transaction.Body.MarshalBinary()
	d.Require().NoError(err)

	d.T().Helper()
	d.Require().NotEmpty(tx.Signatures, "Transaction has no signatures")
	pl := new(api.TxRequest)
	pl.Origin = tx.Transaction.Header.Principal
	pl.Signer.Nonce = tx.Signatures[0].GetTimestamp()
	pl.Signer.Url = tx.Signatures[0].GetSigner()
	pl.Signer.PublicKey = tx.Signatures[0].GetPublicKey()
	pl.Signature = tx.Signatures[0].GetSignature()
	pl.KeyPage.Height = tx.Signatures[0].GetSignerHeight()
	pl.Payload = data

	data, err = pl.MarshalJSON()
	d.Require().NoError(err)

	r := d.api().Execute(context.Background(), data)
	err, _ = r.(error)
	d.Require().NoError(err)
}

func (d *e2eDUT) WaitForTxns(txids ...[]byte) {
	d.T().Helper()
	for _, txid := range txids {
		r, err := d.api().Querier().QueryTx(txid, 10*time.Second, api.QueryOptions{})
		d.Require().NoError(err)

		for _, txid := range r.SyntheticTxids {
			_, err := d.api().Querier().QueryTx(txid[:], 10*time.Second, api.QueryOptions{})
			d.Require().NoError(err)
		}
	}
}
