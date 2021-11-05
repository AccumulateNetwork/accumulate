package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	. "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/genesis"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

var testnet = flag.String("testnet", "Localhost", "TestNet to load test")
var loadWalletCount = flag.Int("loadtest-wallet-count", 10, "Number of wallets")
var loadTxCount = flag.Int("loadtest-tx-count", 10, "Number of transactions")

func TestLoadOnRemote(t *testing.T) {
	t.Skip("Deprecated, use `accumulated loadtest`")

	txBouncer, err := relay.NewWith(*testnet)
	if err != nil {
		t.Fatal(err)
	}

	err = txBouncer.Start()
	require.NoError(t, err)
	defer func() { require.NoError(t, txBouncer.Stop()) }()

	query := NewQuery(txBouncer)
	_, privateKeySponsor, _ := ed25519.GenerateKey(nil)

	addrList, err := acctesting.RunLoadTest(query, privateKeySponsor, *loadWalletCount, *loadTxCount)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10000 * time.Millisecond)

	queryTokenUrl := addrList[1]

	resp, err := query.GetChainStateByUrl(queryTokenUrl)
	if err != nil {
		t.Fatal(err)
	}

	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))

	jsonapi := NewTest(t, query)
	_ = jsonapi

	params := &api.APIRequestURL{URL: types.String(queryTokenUrl)}
	gParams, err := json.Marshal(params)
	theData := jsonapi.GetData(context.Background(), gParams)
	theJsonData, err := json.Marshal(theData)
	if err != nil {
		t.Fatal(err)
	}
	println(string(theJsonData))

	resp, err = query.GetChainStateByUrl(queryTokenUrl)
	if err != nil {
		t.Fatal(err)
	}

	output, err = json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))
	for _, v := range addrList[1:] {
		resp, err := query.GetChainStateByUrl(v)
		if err != nil {
			t.Fatal(err)
		}
		output, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("%s : %s\n", v, string(output))
	}
}

func TestJsonRpcAnonToken(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("This test is flaky in CI")
	}

	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, pv, query := startBVC(t, dir)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	kpSponsor := ed25519.NewKeyFromSeed(pv.Key.PrivKey.Bytes()[:32])

	addrList, err := acctesting.RunLoadTest(query, kpSponsor, *loadWalletCount, *loadTxCount)
	if err != nil {
		t.Fatal(err)
	}

	//wait 3 seconds for the transaction to process for the block to complete.
	time.Sleep(10 * time.Second)

	queryTokenUrl := addrList[1]
	resp, err := query.GetTokenAccount(queryTokenUrl)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(string(*resp.Data))
	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))

	resp2, err := query.GetChainStateByUrl(queryTokenUrl)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(string(*resp.Data))
	output, err = json.Marshal(resp2)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))

	// now use the JSON rpc api's to get the data
	jsonapi := NewTest(t, query)

	params := &api.APIRequestURL{URL: types.String(queryTokenUrl)}
	gParams, err := json.Marshal(params)
	theData := jsonapi.GetData(context.Background(), gParams)
	theJsonData, err := json.Marshal(theData)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(theJsonData) //ret.Response.Value)
	for _, v := range addrList[1:] {
		resp, err := query.GetChainStateByUrl(v)
		if err != nil {
			t.Fatal(err)
		}
		output, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("%s : %s\n", v, string(output))
	}

	//req := api.{}
	//adi := &api.ADI{}
	//adi.URL = "RoadRunner"
	//adi.PublicKeyHash = sha256.Sum256(privateKey.PubKey().Bytes())
	//data, err := json.Marshal(adi)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//req.Tx = &api.APIRequestRawTx{}
	//req.Tx.Signer = &api.Signer{}
	//req.Tx.Signer.URL = types.String(adiSponsor)
	//copy(req.Tx.Signer.PublicKey[:], kpSponsor.PubKey().Bytes())
	//req.Tx.Timestamp = time.Now().Unix()
	//adiJson := json.RawMessage(data)
	//req.Tx.Data = &adiJson
	//
	//ledger := types.MarshalBinaryLedgerAdiChainPath(*adi.URL.AsString(), *req.Tx.Data, req.Tx.Timestamp)
	//sig, err := kpSponsor.Sign(ledger)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//copy(req.Sig[:], sig)
	//
	//jsonReq, err := json.Marshal(&req)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	////now we can send in json rpc calls.
	//ret := jsonapi.faucet(context.Background(), jsonReq)

	//wait 30 seconds before shutting down is useful when debugging the tendermint core callbacks
	time.Sleep(1000 * time.Millisecond)

}

func TestFaucet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	}

	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	_, kpSponsor, _ := ed25519.GenerateKey(nil)

	req := &api.APIRequestURL{}
	req.Wait = true
	req.URL = types.String(anon.GenerateAcmeAddress(kpSponsor.Public().(ed25519.PublicKey)))

	params, err := json.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}

	// Create our two transactions
	k1 := []byte("firstName")
	v1 := []byte("satoshi")
	tx1 := append(k1, append([]byte("="), v1...)...)

	k2 := []byte("lastName")
	v2 := []byte("nakamoto")
	tx2 := append(k2, append([]byte("="), v2...)...)

	gtx := transactions.GenTransaction{}
	gtx.Signature = append(gtx.Signature, &transactions.ED25519Sig{})
	gtx.SigInfo = &transactions.SignatureInfo{}
	gtx.SigInfo.URL = "fakeUrl"
	gtx.Transaction = tx1
	gtx.Signature[0].Sign(54321, kpSponsor, gtx.TransactionHash())
	//changing the nonce will invalidate the signature.
	gtx.SigInfo.Nonce = 1234

	//intentionally send in a bogus transaction
	ti1, _ := query.BroadcastTx(&gtx, nil)
	gtx.Transaction = tx2
	ti2, _ := query.BroadcastTx(&gtx, nil)

	stat := query.BatchSend()
	bs := <-stat
	res1, err := bs.ResolveTransactionResponse(ti1)
	if err != nil {
		t.Fatal(err)
	}
	if res1.Code == 0 {
		t.Fatalf("expecting error code that is non zero")
	}

	errorData, err := json.Marshal(res1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(errorData))

	res2, err := bs.ResolveTransactionResponse(ti2)

	if err != nil {
		t.Fatal(err)
	}
	if res2.Code == 0 {
		t.Fatalf("expecting error code that is non zero")
	}

	jsonapi := NewTest(t, query)

	res := jsonapi.Faucet(context.Background(), params)
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(data))

	//allow the transaction to settle.
	time.Sleep(3 * time.Second)

	//readback the result.
	resp, err := query.GetChainStateByUrl(string(req.URL))
	if err != nil {
		t.Fatal(err)
	}
	ta := response.TokenAccount{}
	if resp.Data == nil {
		t.Fatalf("token account not found in query after faucet transaction")
	}

	err = json.Unmarshal(*resp.Data, &ta)
	if err != nil {
		t.Fatal(err)
	}

	if ta.Balance.String() != "1000000000" {
		t.Fatalf("incorrect balance after faucet transaction")
	}

	//just dump out the response as the api user would see it
	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s\n", string(output))
}

func TestTransactionHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	}

	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	_, kpSponsor, _ := ed25519.GenerateKey(nil)

	req := &api.APIRequestURL{}
	req.URL = types.String(anon.GenerateAcmeAddress(kpSponsor.Public().(ed25519.PublicKey)))

	params, err := json.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}

	jsonapi := NewTest(t, query)

	res := jsonapi.Faucet(context.Background(), params)
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(data))

	//allow the transaction to settle.
	time.Sleep(3 * time.Second)

	jd, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	r2 := api.APIDataResponse{}
	err = json.Unmarshal(jd, &r2)
	if err != nil {
		t.Fatal(err)
	}

	txr := response.TokenTx{}
	err = json.Unmarshal(*r2.Data, &txr)
	if err != nil {
		t.Fatal(err)
	}
	//readback the result.

	d2, err := query.GetTransaction(txr.TxId)
	if err != nil {
		t.Fatal(err)
	}

	//fmt.Println(d2.Data)
	output, err := json.Marshal(d2.Data)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s\n", string(output))
	resp, err := query.GetTransactionHistory(*req.URL.AsString(), 0, 1)
	if err != nil {
		t.Fatal(err)
	}

	//just dump out the response as the api user would see it
	output, err = json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s\n", string(output))
}

func TestFaucetTransactionHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	}

	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	req := &api.APIRequestURL{}
	req.URL = types.String(anon.GenerateAcmeAddress(ed25519.PublicKey{}))
	params, err := json.Marshal(&req)
	require.NoError(t, err)

	_, _, query := startBVC(t, t.TempDir())
	jsonapi := NewTest(t, query)
	res := jsonapi.Faucet(context.Background(), params)

	//allow the transaction to settle.
	time.Sleep(3 * time.Second)

	jd, err := json.Marshal(res)
	require.NoError(t, err)
	r2 := api.APIDataResponse{}
	err = json.Unmarshal(jd, &r2)
	require.NoError(t, err)

	txr := response.TokenTx{}
	err = json.Unmarshal(*r2.Data, &txr)
	require.NoError(t, err)

	resp, err := query.GetTransactionHistory(genesis.FaucetUrl.String(), 0, 10)
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	require.Equal(t, types.String(types.TxTypeSyntheticGenesis.Name()), resp.Data[0].Type)
	require.Equal(t, types.String(types.TxTypeTokenTx.Name()), resp.Data[1].Type)
}

func TestMetrics(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Depends on an external resource, and thus is not appropriate for CI")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)
	japi := NewTest(t, query)

	req, err := json.Marshal(&protocol.MetricsRequest{Metric: "tps", Duration: time.Hour})
	require.NoError(t, err)

	resp := japi.Metrics(context.Background(), req)
	switch r := resp.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case api.APIDataResponse:
		mresp := new(protocol.MetricsResponse)
		require.NoError(t, json.Unmarshal(*r.Data, mresp))
	default:
		require.IsType(t, api.APIDataResponse{}, r)
	}
}

func TestQueryNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Fails on windows (Tendermint cleanup issue)")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("Flaky in CI")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)
	japi := NewTest(t, query)

	req, err := json.Marshal(&api.APIRequestURL{URL: "acc://1cddf368ef9ba2a1ea914291e0201ebaf376130a6c05caf3/ACME"})
	require.NoError(t, err)

	resp := japi.GetTokenAccount(context.Background(), req)
	switch r := resp.(type) {
	case jsonrpc2.Error:
		require.Equal(t, jsonrpc2.ErrorCode(ErrCodeNotFound), r.Code)
	default:
		t.Fatalf("Expected error, got %T", r)
	}
}

func TestQueryWrongType(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Fails on windows (Tendermint cleanup issue)")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)
	japi := NewTest(t, query)

	_, origin, _ := ed25519.GenerateKey(nil)
	destAddress, _, tx, err := acctesting.BuildTestSynthDepositGenTx(origin)
	require.NoError(t, err)

	err = acctesting.SendTxSync(query, tx)
	require.NoError(t, err)

	req, err := json.Marshal(&api.APIRequestURL{URL: destAddress})
	require.NoError(t, err)

	resp := japi.GetADI(context.Background(), req)
	switch r := resp.(type) {
	case jsonrpc2.Error:
		require.Contains(t, r.Data, "want ChainTypeAdi, got ChainTypeAnonTokenAccount")
	default:
		t.Fatalf("Expected error, got %T", r)
	}
}

func TestGetTxId(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Fails on windows (Tendermint cleanup issue)")
	}

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)
	japi := NewTest(t, query)

	_, origin, _ := ed25519.GenerateKey(nil)
	destAddress, _, tx, err := acctesting.BuildTestSynthDepositGenTx(origin)
	require.NoError(t, err)

	err = acctesting.SendTxSync(query, tx)
	require.NoError(t, err)

	u, err := url.Parse(*destAddress.AsString())
	require.NoError(t, err)
	u.Path = ""
	u.Query = fmt.Sprintf("txid=%x", tx.TransactionHash())

	req, err := json.Marshal(&api.APIRequestURL{URL: types.String(u.String())})
	require.NoError(t, err)

	resp := japi.GetData(context.Background(), req)
	switch r := resp.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *api.APIDataResponse:
		// TODO Check response
	default:
		require.IsType(t, (*api.APIDataResponse)(nil), r)
	}
}

func TestDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Fails on windows (Tendermint cleanup issue)")
	}

	dir := t.TempDir()
	db, _, query := startBVC(t, dir)
	japi := NewTest(t, query)

	_, adiKey, _ := ed25519.GenerateKey(nil)
	require.NoError(t, acctesting.CreateADI(db, tmed25519.PrivKey(adiKey), "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(db, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	_, _, err := db.WriteStates(0)
	require.NoError(t, err)

	req, err := json.Marshal(&api.APIRequestURL{URL: "foo"})
	require.NoError(t, err)

	resp := japi.GetDirectory(context.Background(), req)
	switch r := resp.(type) {
	case jsonrpc2.Error:
		require.NoError(t, r)
	case *api.APIDataResponse:
		dir := new(protocol.DirectoryQueryResult)
		require.NoError(t, json.Unmarshal(*r.Data, dir))
		t.Log(dir)
	default:
		require.IsType(t, (*api.APIDataResponse)(nil), r)
	}
}

func TestFaucetReplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	}

	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	_, kpSponsor, _ := ed25519.GenerateKey(nil)
	destAccount := anon.GenerateAcmeAddress(kpSponsor.Public().(ed25519.PublicKey))
	tx := acmeapi.TokenTx{}
	tx.From.String = types.String(genesis.FaucetWallet.Addr)
	tx.AddToAccount(types.String(destAccount), 1000000000)

	genesis.FaucetWallet.Nonce = uint64(time.Now().UnixNano())
	gtx, err := transactions.New(*tx.From.AsString(), func(hash []byte) (*transactions.ED25519Sig, error) {
		return genesis.FaucetWallet.Sign(hash), nil
	}, &tx)
	require.NoError(t, err)

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	_, _, query := startBVC(t, dir)

	jsonapi := NewTest(t, query)
	res := jsonapi.BroadcastTx(false, gtx)
	fmt.Printf("%s\n", *res.Data)

	// Allow the transaction to settle.
	time.Sleep(3 * time.Second)

	// Read back the result.
	resp, err := query.GetChainStateByUrl(destAccount)
	require.NoError(t, err)
	require.NotNil(t, resp.Data, "token account not found in query after faucet transaction")
	ta := response.TokenAccount{}
	require.NoError(t, json.Unmarshal(*resp.Data, &ta))
	require.Equal(t, "1000000000", ta.Balance.String(), "incorrect balance after faucet transaction")

	// Replay
	res = jsonapi.BroadcastTx(false, gtx)
	v := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(*res.Data, &v))

	// https://github.com/tendermint/tendermint/issues/7185
	require.Contains(t, v["error"], "tx already exists in cache")
}
