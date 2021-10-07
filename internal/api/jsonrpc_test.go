package api_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/rpc/client/http"
)

var testnet = flag.String("testnet", "Localhost", "TestNet to load test")
var loadWalletCount = flag.Int("loadtest-wallet-count", 10, "Number of wallets")
var loadTxCount = flag.Int("loadtest-tx-count", 10, "Number of transactions")

func TestLoadOnRemote(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("This test is not appropriate for CI")
	}

	txBouncer, err := relay.NewWith(*testnet)
	if err != nil {
		t.Fatal(err)
	}

	query := NewQuery(txBouncer)
	_, privateKeySponsor, _ := ed25519.GenerateKey(nil)

	addrList, err := acctesting.RunLoadTest(query, &privateKeySponsor, *loadWalletCount, *loadTxCount)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10000 * time.Millisecond)

	queryTokenUrl := addrList[1]

	resp, err := query.GetChainState(&queryTokenUrl, nil)
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

	resp, err = query.GetChainState(&queryTokenUrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err = json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))
	for _, v := range addrList[1:] {
		resp, err := query.GetChainState(&v, nil)
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

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	node, pv := startBVC(t, dir)
	defer node.Stop()

	rpcClient, err := http.New(node.Config.RPC.ListenAddress)
	require.NoError(t, err)
	txBouncer := relay.New(rpcClient)
	query := NewQuery(txBouncer)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	kpSponsor := ed25519.NewKeyFromSeed(pv.Key.PrivKey.Bytes()[:32])

	addrList, err := acctesting.RunLoadTest(query, &kpSponsor, *loadWalletCount, *loadTxCount)
	if err != nil {
		t.Fatal(err)
	}

	//wait 3 seconds for the transaction to process for the block to complete.
	time.Sleep(10 * time.Second)

	queryTokenUrl := addrList[1]
	resp, err := query.GetTokenAccount(&queryTokenUrl)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(string(*resp.Data))
	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))

	resp2, err := query.GetChainState(&queryTokenUrl, nil)
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
		resp, err := query.GetChainState(&v, nil)
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

func TestJsonRpcAdi(t *testing.T) {
	t.Skip("Test Broken") // ToDo: Broken Test

	//"wileecoyote/ACME"
	adiSponsor := "wileecoyote"

	kpNewAdi := types.CreateKeyPair()
	//routerAddress := fmt.Sprintf("tcp://localhost:%d", randomRouterPorts())

	//make a client, and also spin up the router grpc
	dir := t.TempDir()
	node, pv := startBVC(t, dir)
	defer node.Stop()

	//kpSponsor := types.CreateKeyPair()

	rpcClient, err := http.New(node.Config.RPC.ListenAddress)
	require.NoError(t, err)
	txBouncer := relay.New(rpcClient)
	query := NewQuery(txBouncer)
	jsonapi := NewTest(t, query)

	//StartAPI(randomRouterPorts(), client)

	kpSponsor := types.CreateKeyPairFromSeed(pv.Key.PrivKey.Bytes())

	req := api.APIRequestRaw{}
	adi := &api.ADI{}
	adi.URL = "RoadRunner"
	adi.PublicKeyHash = sha256.Sum256(kpNewAdi.PubKey().Bytes())
	data, err := json.Marshal(adi)
	if err != nil {
		t.Fatal(err)
	}

	req.Tx = &api.APIRequestRawTx{}
	req.Tx.Signer = &api.Signer{}
	req.Tx.Signer.URL = types.String(adiSponsor)
	copy(req.Tx.Signer.PublicKey[:], kpSponsor.PubKey().Bytes())
	req.Tx.Timestamp = time.Now().Unix()
	adiJson := json.RawMessage(data)
	req.Tx.Data = &adiJson

	// TODO Why does this sign a ledger? This will fail in GenTransaction.
	ledger := types.MarshalBinaryLedgerAdiChainPath(*adi.URL.AsString(), *req.Tx.Data, req.Tx.Timestamp)
	sig, err := kpSponsor.Sign(ledger)
	if err != nil {
		t.Fatal(err)
	}
	copy(req.Sig[:], sig)

	jsonReq, err := json.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}

	//now we can send in json rpc calls.
	ret := jsonapi.CreateADI(context.Background(), jsonReq)

	t.Fatal(ret)

}
