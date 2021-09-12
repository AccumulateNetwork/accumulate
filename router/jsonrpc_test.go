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

	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/go-playground/validator/v10"
	ptypes "github.com/tendermint/tendermint/abci/types"
)

//
//func sendFaucetTokenDeposit(client, address) {
//
//}

func TestJsonRpcAnonToken(t *testing.T) {

	_, privateKey, _ := ed25519.GenerateKey(nil)

	//make a client, and also spin up the router grpc
	dir, err := ioutil.TempDir("/tmp", "AccRouterTest-")
	cfg := path.Join(dir, "/config/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	client, _, _, _, vm := makeBVCandRouter(t, cfg, dir)

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

	txid := sha256.Sum256([]byte("txid"))

	tokenUrl := types.String("dc/ACME")

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(txid[:], &adiSponsor, &destAddress)
	deposit.DepositAmount.SetInt64(5000)
	deposit.TokenUrl = tokenUrl

	data, err := deposit.MarshalBinary()
	sig := ed25519.Sign(kpSponsor, data)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Instruction(proto.AccInstruction_Synthetic_Token_Deposit).
		Data(data).
		PubKey(types.Bytes(kpSponsor.Public().(ed25519.PublicKey))).
		Timestamp(time.Now().Unix()).
		AdiUrl(*destAddress.AsString()).
		Signature(sig).
		Build()

	_, err = client.ProcessTx(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}

	Load(t, query, privateKey)

	//wait 3 seconds for the transaction to process for the block to complete.
	time.Sleep(3000 * time.Millisecond)
	queryTokenUrl := destAddress + "/" + tokenUrl
	resp, err := query.GetTokenAccount(queryTokenUrl.AsString())
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(string(*resp.Data))
	output, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(output)

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

}

func Load(t *testing.T, query *Query, Origin ed25519.PrivateKey) {
	srcURL := anon.GenerateAcmeAddress(Origin[32:])
	var SetOKeys []ed25519.PrivateKey
	var Addresses []string
	for i := 0; i < 1000; i++ {
		_, key, _ := ed25519.GenerateKey(nil)
		SetOKeys = append(SetOKeys, key)
		Addresses = append(Addresses, anon.GenerateAcmeAddress(key[32:]))
	}
	if len(Addresses) == 0 || len(SetOKeys) == 0 {
		t.Fatal("no addresses")
	}
	for i := 0; i < 100; i++ {
		d := rand.Int() % len(SetOKeys)
		out := proto.Output{Dest: Addresses[d], Amount: 10 * 1000000}
		send := proto.NewTokenSend(srcURL, out)
		txData := send.Marshal()
		gtx := new(proto.GenTransaction)
		gtx.Transaction = txData
		if err := gtx.SetRoutingChainID(Addresses[d]); err != nil {
			t.Fatal("bad url generated")
		}
		s := ed25519.Sign(SetOKeys[d], txData)
		ed := new(proto.ED25519Sig)
		ed.PublicKey = SetOKeys[d][32:]
		ed.Signature = s
		gtx.Signature = append(gtx.Signature, ed)

		deliverRequestTXAsync := new(ptypes.RequestDeliverTx)
		deliverRequestTXAsync.Tx = gtx.Marshal()
		query.client.DeliverTxAsync(*deliverRequestTXAsync)
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

	client, _, _, _, vm := makeBVCandRouter(t, cfg, dir)

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
