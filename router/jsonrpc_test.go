package router

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/go-playground/validator/v10"
)

func TestJsonRpcAnonToken(t *testing.T) {

	kpNewAdi := types.CreateKeyPair()
	_ = kpNewAdi

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

	kpSponsor := types.CreateKeyPairFromSeed(vm.Key.PrivKey.Bytes())

	//use the public key of the bvc...
	adiSponsor := types.GenerateAcmeAddress(kpSponsor.PubKey().Bytes())

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit()
	deposit.DepositAmount.SetInt64(5000)
	deposit.TokenUrl = "dc/ACME"
	deposit.Txid = sha256.Sum256([]byte("txid")) //just use a fake txid
	data, err := deposit.MarshalBinary()
	sig, err := kpSponsor.Sign(data)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Instruction(proto.AccInstruction_Synthetic_Token_Deposit).
		Data(data).
		PubKey(kpSponsor.PubKey().Bytes()).
		Timestamp(time.Now().Unix()).
		AdiUrl(adiSponsor).
		Signature(sig).
		Build()

	//query
	//req := api.{}
	//adi := &api.ADI{}
	//adi.URL = "RoadRunner"
	//adi.PublicKeyHash = sha256.Sum256(kpNewAdi.PubKey().Bytes())
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
	//ret := jsonapi.createADI(context.Background(), jsonReq)

	_, err = client.ProcessTx(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
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
