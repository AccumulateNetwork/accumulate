package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/AccumulateNetwork/accumulate/internal/api"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/response"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

func TestJsonRpcLiteToken(t *testing.T) {
	t.Skip("This test is broken due to changes in how nonces are handled")

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()

	addrList, err := acctesting.RunLoadTest(query, 10, 10)
	if err != nil {
		t.Fatal(err)
	}

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
	jsonapi := NewTest(t, &daemon.Config.Accumulate.API, query)

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
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()

	//create a key from the Tendermint node's private key. He will be the defacto source for the lite token.
	_, kpSponsor, _ := ed25519.GenerateKey(nil)

	req := &api.APIRequestURL{}
	req.Wait = true
	req.URL = types.String(acctesting.AcmeLiteAddressStdPriv(kpSponsor).String())

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

	gtx := transactions.Envelope{}
	gtx.Transaction = new(transactions.Transaction)
	gtx.Signatures = append(gtx.Signatures, &transactions.ED25519Sig{})
	gtx.Transaction.Origin = &url.URL{Authority: "fakeUrl"}
	gtx.Transaction.Body = tx1
	gtx.Signatures[0].Sign(54321, kpSponsor, gtx.Transaction.Hash())
	//changing the nonce will invalidate the signature.
	gtx.Transaction.Nonce = 1234

	//intentionally send in a bogus transaction
	ti1, _ := query.BroadcastTx(&gtx, nil)
	gtx.Transaction.Body = tx2
	ti2, _ := query.BroadcastTx(&gtx, nil)

	stat := query.BatchSend()
	bs := <-stat
	res1, err := bs.ResolveTransactionResponse(ti1)
	require.NoError(t, err)
	require.NotZero(t, res1.Code, "expecting error code that is non zero")

	res2, err := bs.ResolveTransactionResponse(ti2)
	require.NoError(t, err)
	require.NotZero(t, res2.Code, "expecting error code that is non zero")

	jsonapi := NewTest(t, &daemon.Config.Accumulate.API, query)

	res := jsonapi.Faucet(context.Background(), params)
	require.IsType(t, (*api.APIDataResponse)(nil), res)
	require.NoError(t, acctesting.WaitForTxV1(query, res.(*acmeapi.APIDataResponse)))

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
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()

	//create a key from the Tendermint node's private key. He will be the defacto source for the lite token.
	_, kpSponsor, _ := ed25519.GenerateKey(nil)

	req := &api.APIRequestURL{}
	req.URL = types.String(acctesting.AcmeLiteAddressStdPriv(kpSponsor).String())

	params, err := json.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}

	jsonapi := NewTest(t, &daemon.Config.Accumulate.API, query)

	res := jsonapi.Faucet(context.Background(), params)
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(data))

	//allow the transaction to settle.
	time.Sleep(time.Second)

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
	req := &api.APIRequestURL{}
	req.URL = types.String(acctesting.AcmeLiteAddress(ed25519.PublicKey{}).String())
	params, err := json.Marshal(&req)
	require.NoError(t, err)

	daemon := startBVC(t, t.TempDir())
	query := daemon.Query_TESTONLY()
	jsonapi := NewTest(t, &daemon.Config.Accumulate.API, query)
	res := jsonapi.Faucet(context.Background(), params)
	if err, ok := res.(error); ok {
		require.NoError(t, err)
	}

	//allow the transaction to settle.
	time.Sleep(time.Second)

	jd, err := json.Marshal(res)
	require.NoError(t, err)
	r2 := api.APIDataResponse{}
	err = json.Unmarshal(jd, &r2)
	require.NoError(t, err)

	txr := response.TokenTx{}
	err = json.Unmarshal(*r2.Data, &txr)
	require.NoError(t, err)

	resp, err := query.GetTransactionHistory(protocol.FaucetUrl.String(), 0, 10)
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	require.Equal(t, types.String(types.TxTypeInternalGenesis.Name()), resp.Data[0].Type)
	require.Equal(t, types.String(types.TxTypeSendTokens.Name()), resp.Data[1].Type)
}

func TestMetrics(t *testing.T) {
	acctesting.SkipCI(t, "depends on an external resource")

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()
	japi := NewTest(t, &daemon.Config.Accumulate.API, query)

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
	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()
	japi := NewTest(t, &daemon.Config.Accumulate.API, query)

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
	acctesting.SkipCI(t, "deprecated")

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()
	japi := NewTest(t, &daemon.Config.Accumulate.API, query)

	destAddress, _, tx, err := acctesting.BuildTestSynthDepositGenTx()
	require.NoError(t, err)

	err = acctesting.SendTxSync(query, tx)
	require.NoError(t, err)

	req, err := json.Marshal(&api.APIRequestURL{URL: destAddress})
	require.NoError(t, err)

	resp := japi.GetADI(context.Background(), req)
	switch r := resp.(type) {
	case jsonrpc2.Error:
		require.Contains(t, r.Data, "want identity, got liteTokenAccount")
	default:
		t.Fatalf("Expected error, got %T", r)
	}
}

func TestGetTxId(t *testing.T) {
	acctesting.SkipCI(t, "deprecated")

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()
	japi := NewTest(t, &daemon.Config.Accumulate.API, query)

	destAddress, _, tx, err := acctesting.BuildTestSynthDepositGenTx()
	require.NoError(t, err)

	err = acctesting.SendTxSync(query, tx)
	require.NoError(t, err)

	u, err := url.Parse(*destAddress.AsString())
	require.NoError(t, err)
	u.Path = ""
	u.Query = fmt.Sprintf("txid=%x", tx.Transaction.Hash())

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
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()
	japi := NewTest(t, &daemon.Config.Accumulate.API, query)

	_, adiKey, _ := ed25519.GenerateKey(nil)
	dbTx := daemon.DB_TESTONLY().Begin()
	require.NoError(t, acctesting.CreateADI(dbTx, tmed25519.PrivKey(adiKey), "foo"))
	require.NoError(t, acctesting.CreateTokenAccount(dbTx, "foo/tokens", protocol.AcmeUrl().String(), 1, false))
	require.NoError(t, dbTx.Commit())

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
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	_, kpSponsor, _ := ed25519.GenerateKey(nil)
	destAccount := acctesting.AcmeLiteAddressStdPriv(kpSponsor).String()
	tx := protocol.SendTokens{}
	tx.AddRecipient(acctesting.MustParseUrl(destAccount), 1000000000)

	protocol.FaucetWallet.Nonce = uint64(time.Now().UnixNano())
	gtx, err := transactions.New(protocol.FaucetWallet.Addr, 1, func(hash []byte) (*transactions.ED25519Sig, error) {
		return protocol.FaucetWallet.Sign(hash), nil
	}, &tx)
	require.NoError(t, err)

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	daemon := startBVC(t, dir)
	query := daemon.Query_TESTONLY()

	jsonapi := NewTest(t, &daemon.Config.Accumulate.API, query)
	res, err := jsonapi.BroadcastTx(false, gtx)
	require.NoError(t, err)
	require.NoError(t, acctesting.WaitForTxV1(query, res))

	// Allow the transaction to settle.
	time.Sleep(time.Second)

	// Read back the result.
	resp, err := query.GetChainStateByUrl(destAccount)
	require.NoError(t, err)
	require.NotNil(t, resp.Data, "token account not found in query after faucet transaction")
	ta := response.TokenAccount{}
	require.NoError(t, json.Unmarshal(*resp.Data, &ta))
	require.Equal(t, "1000000000", ta.Balance.String(), "incorrect balance after faucet transaction")

	// Replay - see https://github.com/tendermint/tendermint/issues/7185
	_, err = jsonapi.BroadcastTx(false, gtx)
	require.IsType(t, jsonrpc2.Error{}, err)
	jerr := err.(jsonrpc2.Error)
	require.Equal(t, jsonrpc2.ErrorCode(ErrCodeDuplicateTxn), jerr.Code, jerr.Message)
}
