package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/testing/e2e"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
)

var reAlphaNum = regexp.MustCompile("[^a-zA-Z0-9]")

func startAccumulate(t *testing.T, ips []net.IP, bvns, validators, basePort int) []*accumulated.Daemon {
	if len(ips) != bvns*validators {
		panic(fmt.Errorf("want %d validators each for %d BVNs but got %d IPs", validators, bvns, len(ips)))
	}

	names := make([]string, bvns)
	addrs := make(map[string][]string, bvns)
	IPs := make([][]string, bvns)
	config := make([][]*config.Config, bvns)

	workDir := t.TempDir()
	name := reAlphaNum.ReplaceAllString(t.Name(), "-")
	for bvn := 0; bvn < bvns; bvn++ {
		names[bvn] = fmt.Sprintf("%s-BVN%d", name, bvn)
		IPs[bvn] = make([]string, validators)
		addrs[names[bvn]] = make([]string, validators)
		for val := 0; val < validators; val++ {
			ip := ips[0]
			ips = ips[1:]
			IPs[bvn][val] = ip.String()
			addrs[names[bvn]][val] = fmt.Sprintf("http://%v:%d", ip, basePort)
		}

		config[bvn] = make([]*cfg.Config, validators)
		for val := 0; val < validators; val++ {
			config[bvn][val] = acctesting.DefaultConfig(cfg.BlockValidator, cfg.Validator, names[bvn])
			config[bvn][val].Accumulate.Network.BvnNames = names
			config[bvn][val].Accumulate.Network.Addresses = addrs
		}
	}

	daemons := make([]*accumulated.Daemon, 0, bvns*validators)
	for bvn := 0; bvn < bvns; bvn++ {
		require.NoError(t, node.Init(node.InitOptions{
			WorkDir:  path.Join(workDir, fmt.Sprintf("bvn%d", bvn)),
			Port:     3000,
			Config:   config[bvn],
			RemoteIP: IPs[bvn],
			ListenIP: IPs[bvn],
			Logger:   logging.NewTestLogger(t, "plain", cfg.DefaultLogLevels, false),
		}))

		for val := 0; val < validators; val++ {
			daemon, err := acctesting.RunDaemon(acctesting.DaemonOptions{
				Dir:       filepath.Join(workDir, fmt.Sprintf("bvn%d", bvn), fmt.Sprintf("Node%d", val)),
				LogWriter: logging.TestLogWriter(t),
			}, t.Cleanup)
			require.NoError(t, err)
			daemon.Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) { require.NoError(t, err) })
			daemons = append(daemons, daemon)
		}
	}

	return daemons
}

func newKey(seed []byte) ed25519.PrivateKey {
	h := sha256.Sum256(seed)
	return ed25519.NewKeyFromSeed(h[:])
}

func makeLiteUrl(t *testing.T, key ed25519.PrivateKey, tok string) *url.URL {
	t.Helper()
	u, err := protocol.LiteAddress(key[32:], tok)
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

func query(t *testing.T, japi *api.JrpcMethods, method string, params interface{}) *api.ChainQueryResponse {
	t.Helper()

	r := new(api.ChainQueryResponse)
	callApi(t, japi, method, params, r)
	return r
}

func queryAs(t *testing.T, japi *api.JrpcMethods, method string, params, result interface{}) {
	t.Helper()
	r := query(t, japi, method, params)
	recode(t, r.Data, result)
}

type execParams struct {
	Origin    string
	Key       ed25519.PrivateKey
	PageIndex uint64
	Payload   protocol.TransactionPayload
}

func recode(t *testing.T, from, to interface{}) {
	t.Helper()

	b, err := json.Marshal(from)
	require.NoError(t, err)
	err = json.Unmarshal(b, to)
	require.NoError(t, err)
}

func executeTx(t *testing.T, japi *api.JrpcMethods, method string, wait bool, params execParams) *api.TxResponse {
	t.Helper()

	qr := query(t, japi, "query", &api.UrlQuery{Url: params.Origin})
	now := time.Now()
	nonce := uint64(now.Unix()*1e9) + uint64(now.Nanosecond())
	tx, err := transactions.NewWith(&transactions.SignatureInfo{
		URL:           params.Origin,
		KeyPageIndex:  params.PageIndex,
		KeyPageHeight: qr.MainChain.Height,
		Nonce:         nonce,
	}, func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(nonce, params.Key, hash)
	}, params.Payload)
	require.NoError(t, err)

	req := new(api.TxRequest)
	req.Origin = params.Origin
	req.Signer.PublicKey = params.Key[32:]
	req.Signer.Nonce = nonce
	req.Signature = tx.Signature[0].Signature
	req.KeyPage.Index = params.PageIndex
	req.KeyPage.Height = qr.MainChain.Height
	req.Payload = params.Payload

	r := new(api.TxResponse)
	callApi(t, japi, method, req, r)
	require.Zero(t, r.Code, r.Message)

	if wait {
		txWait(t, japi, r.Txid)
	}
	return r
}

func executeTxFail(t *testing.T, japi *api.JrpcMethods, method string, height uint64, params execParams) *api.TxResponse {
	t.Helper()

	now := time.Now()
	nonce := uint64(now.Unix()*1e9) + uint64(now.Nanosecond())
	tx, err := transactions.NewWith(&transactions.SignatureInfo{
		URL:           params.Origin,
		KeyPageIndex:  params.PageIndex,
		KeyPageHeight: height,
		Nonce:         nonce,
	}, func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(nonce, params.Key, hash)
	}, params.Payload)
	require.NoError(t, err)

	req := new(api.TxRequest)
	req.Origin = params.Origin
	req.Signer.PublicKey = params.Key[32:]
	req.Signer.Nonce = nonce
	req.Signature = tx.Signature[0].Signature
	req.KeyPage.Index = params.PageIndex
	req.KeyPage.Height = height
	req.Payload = params.Payload

	r := new(api.TxResponse)
	callApi(t, japi, method, req, r)
	return r
}

func txWait(t *testing.T, japi *api.JrpcMethods, txid []byte) {
	t.Helper()

	txr := query(t, japi, "query-tx", &api.TxnQuery{Txid: txid, Wait: 10 * time.Second})
	for _, txid := range txr.SyntheticTxids {
		query(t, japi, "query-tx", &api.TxnQuery{Txid: txid[:], Wait: 10 * time.Second})
	}
}

type e2eDUT struct {
	*e2e.Suite
	daemons []*accumulated.Daemon
}

func (d *e2eDUT) api() *api.JrpcMethods {
	return d.daemons[0].Jrpc_TESTONLY()
}

func (d *e2eDUT) GetRecordAs(url string, target state.Chain) {
	r, err := d.api().Querier().QueryUrl(url)
	d.Require().NoError(err)
	d.Require().IsType(target, r.Data)
	reflect.ValueOf(target).Elem().Set(reflect.ValueOf(r.Data).Elem())
}

func (d *e2eDUT) GetRecordHeight(url string) uint64 {
	r, err := d.api().Querier().QueryUrl(url)
	d.Require().NoError(err)
	return r.MainChain.Height
}

func (d *e2eDUT) SubmitTxn(tx *transactions.GenTransaction) {
	d.Require().NotEmpty(tx.Signature, "Transaction has no signatures")
	pl := new(api.TxRequest)
	pl.Origin = tx.SigInfo.URL
	pl.Signer.Nonce = tx.SigInfo.Nonce
	pl.Signer.PublicKey = tx.Signature[0].PublicKey
	pl.Signature = tx.Signature[0].Signature
	pl.KeyPage.Index = tx.SigInfo.KeyPageIndex
	pl.KeyPage.Height = tx.SigInfo.KeyPageHeight
	pl.Payload = tx.Transaction

	data, err := pl.MarshalJSON()
	d.Require().NoError(err)

	r := d.api().Execute(context.Background(), data)
	err, _ = r.(error)
	d.Require().NoError(err)
}

func (d *e2eDUT) WaitForTxns(txids ...[]byte) {
	for _, txid := range txids {
		r, err := d.api().Querier().QueryTx(txid, 10*time.Second)
		d.Require().NoError(err)

		for _, txid := range r.SyntheticTxids {
			_, err := d.api().Querier().QueryTx(txid[:], 10*time.Second)
			d.Require().NoError(err)
		}
	}
}
