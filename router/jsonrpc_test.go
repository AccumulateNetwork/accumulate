package router

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/testing"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/go-playground/validator/v10"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
)

var testnet = flag.Int("testnet", 4, "TestNet to load test")
var loadWalletCount = flag.Int("loadtest-wallet-count", 100, "Number of wallets")
var loadTxCount = flag.Int("loadtest-tx-count", 10, "Number of transactions per wallet")

func TestLoadOnRemote(t *testing.T) {
	txBouncer := relay.NewWithNetworks(*testnet)

	_, privateKeySponsor, _ := ed25519.GenerateKey(nil)

	addrList := runLoadTest(t, txBouncer, &privateKeySponsor)

	time.Sleep(10000 * time.Millisecond)

	tokenUrl := "dc/ACME"
	queryTokenUrl := addrList[1] + "/" + tokenUrl
	query := NewQuery(txBouncer)

	resp, err := query.GetChainState(&queryTokenUrl, nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))

	jsonapi := API{randomRouterPorts(), validator.New(), query, txBouncer}
	_ = jsonapi

	params := &api.APIRequestURL{URL: types.String(queryTokenUrl)}
	gParams, err := json.Marshal(params)
	theData := jsonapi.getData(context.Background(), gParams)
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

func runLoadTest(t *testing.T, txBouncer *relay.Relay, origin *ed25519.PrivateKey) (addrList []string) {

	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	adiSponsor := types.String(anon.GenerateAcmeAddress(origin.Public().(ed25519.PublicKey)))

	_, privateKey, _ := ed25519.GenerateKey(nil)
	//set destination url address
	destAddress := types.String(anon.GenerateAcmeAddress(privateKey.Public().(ed25519.PublicKey)))

	gtx, err := acctesting.NewAcmeWalletTx(*origin, privateKey, []byte("fake txid"), "dc/ACME")
	if err != nil {
		t.Fatal(err)
	}

	_, err = txBouncer.SendTx(gtx)
	if err != nil {
		t.Fatal(err)
	}

	var c int
	count, err := acctesting.Load(func(gtx *transactions.GenTransaction) (*coretypes.ResultBroadcastTx, error) {
		c++
		if c%200 == 0 {
			t.Log("Sending batch")
			txBouncer.BatchSend()
			time.Sleep(200 * time.Millisecond)
		}
		return txBouncer.BatchTx(gtx)
	}, privateKey, *loadWalletCount, *loadWalletCount**loadTxCount)

	if err != nil {
		t.Fatal(err)
	}
	txBouncer.BatchSend()

	addresses := make([]string, len(count))
	for addr, ct := range count {
		addrList = append(addrList, addr)
		fmt.Printf("%s : %d\n", addr, ct*1000)
	}

	addrList = append(addrList, *adiSponsor.AsString())
	addrList = append(addrList, *destAddress.AsString())
	addrList = append(addrList, addresses...)
	return addrList

}

func _TestJsonRpcAnonToken(t *testing.T) {
	//make a client, and also spin up the router grpc
	dir, err := ioutil.TempDir("", "AccRouterTest-")
	cfg := filepath.Join(dir, "Node0", "config", "config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	node := startBVC(cfg, dir)

	txBouncer := relay.NewWithNetworks(*testnet)
	if err != nil {
		t.Fatal(err)
	}

	query := NewQuery(txBouncer)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	kpSponsor := ed25519.NewKeyFromSeed(node.PV.Key.PrivKey.Bytes()[:32])

	addrList := runLoadTest(t, txBouncer, &kpSponsor)

	//wait 3 seconds for the transaction to process for the block to complete.
	time.Sleep(10 * time.Second)

	tokenUrl := "dc/ACME"
	queryTokenUrl := addrList[1] + "/" + tokenUrl
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
	jsonapi := API{randomRouterPorts(), validator.New(), query, txBouncer}

	params := &api.APIRequestURL{URL: types.String(queryTokenUrl)}
	gParams, err := json.Marshal(params)
	theData := jsonapi.getData(context.Background(), gParams)
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
	txBouncer := relay.NewWithNetworks(*testnet)

	//"wileecoyote/ACME"
	adiSponsor := "wileecoyote"

	kpNewAdi := types.CreateKeyPair()
	//routerAddress := fmt.Sprintf("tcp://localhost:%d", RandPort())

	//make a client, and also spin up the router grpc
	dir, err := ioutil.TempDir("/tmp", "AccRouterTest-")
	cfg := path.Join(dir, "/config/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	node := startBVC(cfg, dir)

	if err != nil {
		t.Fatal(err)
	}

	//kpSponsor := types.CreateKeyPair()

	query := NewQuery(txBouncer)

	jsonapi := API{randomRouterPorts(), validator.New(), query, txBouncer}

	//StartAPI(RandPort(), client)

	kpSponsor := types.CreateKeyPairFromSeed(node.PV.Key.PrivKey.Bytes())

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
	ret := jsonapi.createADI(context.Background(), jsonReq)

	t.Fatal(ret)

}
