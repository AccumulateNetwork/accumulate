package router

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/go-playground/validator/v10"
)

var testnet = flag.Int("testnet", 4, "TestNet to load test")

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

	txid := sha256.Sum256([]byte("fake txid"))

	tokenUrl := types.String("dc/ACME")

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(txid[:], &adiSponsor, &destAddress)
	amtToDeposit := int64(50000)                             //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * 100000000) // assume 8 decimal places
	deposit.TokenUrl = tokenUrl

	depData, err := deposit.MarshalBinary()
	gtx := new(transactions.GenTransaction)
	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.Transaction = depData
	gtx.SigInfo.URL = *destAddress.AsString()
	gtx.ChainID = types.GetChainIdFromChainPath(destAddress.AsString())[:]
	gtx.Routing = types.GetAddressFromIdentity(destAddress.AsString())

	ed := new(transactions.ED25519Sig)
	gtx.SigInfo.Nonce = 1
	ed.PublicKey = privateKey[32:]
	err = ed.Sign(gtx.SigInfo.Nonce, privateKey, gtx.TransactionHash())
	if err != nil {
		t.Fatal(err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	_, err = txBouncer.SendTx(gtx)

	if err != nil {
		t.Fatal(err)
	}

	addresses := Load(t, txBouncer, privateKey)

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

	_, rpc, vm := makeBVCandRouter(cfg, dir)
	_ = rpc

	txBouncer := relay.NewWithNetworks(*testnet)
	if err != nil {
		t.Fatal(err)
	}

	query := NewQuery(txBouncer)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	kpSponsor := ed25519.NewKeyFromSeed(vm.Key.PrivKey.Bytes()[:32])

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

// Load
// Generate load in our test.  Create a bunch of transactions, and submit them.
func Load(t *testing.T,
	txBouncer *relay.Relay,
	Origin ed25519.PrivateKey) (addrList []string) {

	var wallet []*transactions.WalletEntry

	wallet = append(wallet, transactions.NewWalletEntry()) // wallet[0] is where we put 5000 ACME tokens
	wallet[0].Nonce = 1                                    // start the nonce at 1
	wallet[0].PrivateKey = Origin                          // Put the private key for the origin
	wallet[0].Addr = anon.GenerateAcmeAddress(Origin[32:]) // Generate the origin address

	for i := 0; i < 100; i++ { //                            create a 1000 addresses for anonymous token chains
		wallet = append(wallet, transactions.NewWalletEntry()) // create a new wallet entry
	}

	addrCountMap := make(map[string]int)
	for i := 0; i < 10*len(wallet); i++ { // Make a bunch of transactions
		if i%200 == 0 {
			txBouncer.BatchSend()
			time.Sleep(200 * time.Millisecond)
		}
		const origin = 0
		randDest := rand.Int()%(len(wallet)-1) + 1                            // pick a destination address
		out := transactions.Output{Dest: wallet[randDest].Addr, Amount: 1000} // create the transaction output
		addrCountMap[wallet[randDest].Addr]++                                 // count the number of deposits to output
		send := transactions.NewTokenSend(wallet[origin].Addr, out)           // Create a send token transaction
		gtx := new(transactions.GenTransaction)                               // wrap in a GenTransaction
		gtx.SigInfo = new(transactions.SignatureInfo)                         // Get a Signature Info block
		gtx.Transaction = send.Marshal()                                      // add  send transaction
		gtx.SigInfo.URL = wallet[origin].Addr                                 // URL of source
		if err := gtx.SetRoutingChainID(); err != nil {                       // Routing ChainID is the tx source
			t.Fatal("bad url generated") // error should never happen
		}

		binaryGtx := gtx.TransactionHash() // Must sign the GenTransaction

		gtx.Signature = append(gtx.Signature, wallet[origin].Sign(binaryGtx))

		if resp, err := txBouncer.BatchTx(gtx); err != nil {
			t.Fatal(err) //                                                 <= should never happen
		} else {
			if len(resp.Log) > 0 {
				fmt.Printf("<%d>%v<<\n", i, resp.Log)
			}
		}
	}
	txBouncer.BatchSend()
	for addr, ct := range addrCountMap {
		addrList = append(addrList, addr)
		_ = ct
		fmt.Printf("%s : %d\n", addr, ct*1000)
	}
	return addrList
}

func TestJsonRpcAdi(t *testing.T) {
	txBouncer := relay.NewWithNetworks(*testnet)

	//"wileecoyote/ACME"
	adiSponsor := "wileecoyote"

	kpNewAdi := types.CreateKeyPair()
	//routerAddress := fmt.Sprintf("tcp://localhost:%d", randomRouterPorts())

	//make a client, and also spin up the router grpc
	dir, err := ioutil.TempDir("/tmp", "AccRouterTest-")
	cfg := path.Join(dir, "/config/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	_, _, vm := makeBVCandRouter(cfg, dir)

	if err != nil {
		t.Fatal(err)
	}

	//kpSponsor := types.CreateKeyPair()

	query := NewQuery(txBouncer)

	jsonapi := API{randomRouterPorts(), validator.New(), query, txBouncer}

	//StartAPI(randomRouterPorts(), client)

	kpSponsor := types.CreateKeyPairFromSeed(vm.Key.PrivKey.Bytes())

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
