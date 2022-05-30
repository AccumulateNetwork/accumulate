package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
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
	u, err := protocol.LiteTokenAddress(key[32:], tok, protocol.SignatureTypeED25519)
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
		signator = resp.Signer
	}

	qr := queryRecord(t, japi, "query", &api.UrlQuery{Url: signator})
	env := acctesting.NewTransaction().
		WithPrincipal(u).
		WithSigner(signator, qr.MainChain.Height).
		WithCurrentTimestamp().
		WithBody(params.Payload).
		Initiate(protocol.SignatureTypeLegacyED25519, params.Key)
	sig := env.Signatures[0].(protocol.KeySignature)

	req := new(api.TxRequest)
	req.Origin = env.Transaction[0].Header.Principal
	req.Signer.Timestamp = sig.GetTimestamp()
	req.Signer.Url = sig.GetSigner()
	req.Signer.PublicKey = sig.(protocol.KeySignature).GetPublicKey()
	req.Signature = sig.GetSignature()
	req.KeyPage.Version = sig.GetSignerVersion()
	req.Payload = env.Transaction[0].Body
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
		WithCurrentTimestamp().
		WithBody(params.Payload).
		Initiate(protocol.SignatureTypeLegacyED25519, params.Key)
	sig := env.Signatures[0].(protocol.KeySignature)

	req := new(api.TxRequest)
	req.Origin = env.Transaction[0].Header.Principal
	req.Signer.Timestamp = sig.GetTimestamp()
	req.Signer.Url = sig.GetSigner()
	req.Signer.PublicKey = sig.(protocol.KeySignature).GetPublicKey()
	req.Signature = sig.GetSignature()
	req.KeyPage.Version = sig.GetSignerVersion()
	req.Payload = env.Transaction[0].Body

	resp := new(api.TxResponse)
	callApi(t, japi, method, req, resp)
	return resp
}

func txWait(t *testing.T, japi *api.JrpcMethods, txid []byte) {
	t.Helper()

	txr := queryTxn(t, japi, "query-tx", &api.TxnQuery{Txid: txid, Wait: 10 * time.Second})
	for _, txid := range txr.SyntheticTxids {
		queryTxn(t, japi, "query-tx", &api.TxnQuery{Txid: txid[:], Wait: 10 * time.Second}) //nolint:rangevarref
	}
}
