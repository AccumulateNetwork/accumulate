package router

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types/api/transactions"

	"github.com/AccumulateNetwork/accumulated/networks"

	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/go-playground/validator/v10"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

//RouterNode{
//Name: "Arches",
//Port: 33000,
//Ip: []string{
//"13.51.10.110",
//"13.232.230.216",
//},
//},
//RouterNode{
//Name: "AmericanSamoa",
//Port: 33000,
//Ip: []string{
//"18.221.39.36",
//"44.236.45.58",
//},

func makeBouncer() *networks.Bouncer {
	//laddr := []string { "tcp://18.221.39.36:33001", "tcp://44.236.45.58:33001","tcp://13.51.10.110:33001", "tcp://13.232.230.216:33001" }
	lAddr := []string{"tcp://18.221.39.36:33001", "tcp://13.51.10.110:33001"}

	rpcClients := []*rpchttp.HTTP{}

	rpcClient1, _ := rpchttp.New(lAddr[0], "/websocket")
	rpcClient2, _ := rpchttp.New(lAddr[1], "/websocket")
	rpcClients = append(rpcClients, rpcClient1)
	rpcClients = append(rpcClients, rpcClient2)
	txBouncer := networks.NewBouncer(rpcClients)
	return txBouncer
}

func _TestLoadOnRemote(t *testing.T) {

	txBouncer := makeBouncer()

	_, privateKeySponsor, _ := ed25519.GenerateKey(nil)
	_, privateKey, _ := ed25519.GenerateKey(nil)

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	kpSponsor := privateKeySponsor

	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	adiSponsor := types.String(anon.GenerateAcmeAddress(kpSponsor.Public().(ed25519.PublicKey)))

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
	gtx.Transaction = depData
	gtx.SigInfo.URL = *destAddress.AsString()
	if err := gtx.SetRoutingChainID(); err != nil {
		t.Fatal("bad url generated")
	}
	dataToSign := gtx.TransactionHash()

	ed := new(transactions.ED25519Sig)
	gtx.SigInfo.Nonce = 1
	ed.PublicKey = privateKey[32:]
	err = ed.Sign(gtx.SigInfo.Nonce, privateKey, dataToSign)
	if err != nil {
		t.Fatal(err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	_, _ = txBouncer.SendTx(gtx)

	Load(t, txBouncer, privateKey)

	txBouncer.BatchSend()

}

func TestJsonRpcAnonToken(t *testing.T) {

	_, privateKey, _ := ed25519.GenerateKey(nil)

	//make a client, and also spin up the router grpc
	dir, err := ioutil.TempDir("/tmp", "AccRouterTest-")
	cfg := path.Join(dir, "/config/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	client, _, _, rpcClient, vm := makeBVCandRouter(cfg, dir)

	rpcClients := []*rpchttp.HTTP{rpcClient}
	txBouncer := networks.NewBouncer(rpcClients)

	if err != nil {
		t.Fatal(err)
	}

	query := NewQuery(vm)

	jsonapi := API{RandPort(), validator.New(), client, query}
	_ = jsonapi

	//create a key from the Tendermint node's private key. He will be the defacto source for the anon token.
	kpSponsor := ed25519.NewKeyFromSeed(vm.Key.PrivKey.Bytes()[:32])

	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	adiSponsor := types.String(anon.GenerateAcmeAddress(kpSponsor.Public().(ed25519.PublicKey)))

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
	if err := gtx.SetRoutingChainID(); err != nil {
		t.Fatal("bad url generated")
	}

	ed := new(transactions.ED25519Sig)
	gtx.SigInfo.Nonce = 1
	ed.PublicKey = privateKey[32:]
	err = ed.Sign(gtx.SigInfo.Nonce, privateKey, gtx.TransactionHash())
	if err != nil {
		t.Fatal(err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	if _, err := txBouncer.BatchTx(gtx); err != nil {
		t.Fatal(err)
	}

	Load(t, txBouncer, privateKey)

	txBouncer.BatchSend()

	//wait 3 seconds for the transaction to process for the block to complete.
	time.Sleep(3 * time.Second)
	queryTokenUrl := destAddress + "/" + tokenUrl
	resp, err := query.GetTokenAccount(queryTokenUrl.AsString())
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(string(*resp.Data))
	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(output))

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

	//wait 30 seconds before shutting down.
	time.Sleep(10000 * time.Millisecond)

}

type walletEntry struct {
	PrivateKey ed25519.PrivateKey // 32 bytes private key, 32 bytes public key
	Nonce      uint64             // Nonce for the signature
	Addr       string             // The address url for the anonymous token chain
}

// Sign
// Makes it easier to sign transactions.  Create the ED25519Sig object, sign
// the message, and return the ED25519Sig object to caller
func (we *walletEntry) Sign(message []byte) *transactions.ED25519Sig { // sign a message
	we.Nonce++                                 //                         Everytime we sign, increment the nonce
	sig := new(transactions.ED25519Sig)        //                         create a signature object
	sig.Sign(we.Nonce, we.PrivateKey, message) //                         sign the message
	return sig                                 //                         return the signature object
}

func (we *walletEntry) Public() []byte {
	return we.PrivateKey[32:]
}

// Load
// Generate load in our test.  Create a bunch of transactions, and submit them.
func Load(t *testing.T,
	txBouncer *networks.Bouncer,
	Origin ed25519.PrivateKey) {

	var wallet []*walletEntry

	wallet = append(wallet, new(walletEntry))              // wallet[0] is where we put 5000 ACME tokens
	wallet[0].Nonce = 1                                    // start the nonce at 1
	wallet[0].PrivateKey = Origin                          // Put the private key for the origin
	wallet[0].Addr = anon.GenerateAcmeAddress(Origin[32:]) // Generate the origin address

	for i := 1; i <= 2000; i++ { //                            create a 1000 addresses for anonymous token chains
		wallet = append(wallet, new(walletEntry))                            // create a new wallet entry
		wallet[i].Nonce = 1                                                  // starting nonce of 1
		_, wallet[i].PrivateKey, _ = ed25519.GenerateKey(nil)                // generate a private key
		wallet[i].Addr = anon.GenerateAcmeAddress(wallet[i].PrivateKey[32:]) // generate the address encoding URL
	}

	for i := 1; i < 100000; i++ { // Make a bunch of transactions
		if i%100 == 0 {
			txBouncer.BatchSend()
			time.Sleep(250 * time.Millisecond)
		}
		const origin = 0
		randDest := rand.Int()%(len(wallet)-1) + 1                            // pick a destination address
		out := transactions.Output{Dest: wallet[randDest].Addr, Amount: 1000} // create the transaction output
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
}

func _TestJsonRpcAdi(t *testing.T) {

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

	client, _, _, _, vm := makeBVCandRouter(cfg, dir)

	if err != nil {
		t.Fatal(err)
	}

	//kpSponsor := types.CreateKeyPair()

	query := NewQuery(vm)

	jsonapi := API{RandPort(), validator.New(), client, query}

	//StartAPI(RandPort(), client)

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
