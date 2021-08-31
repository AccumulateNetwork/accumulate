package router

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/go-playground/validator/v10"
	"testing"
	"time"
)

func TestJsonRpcAdi(t *testing.T) {

	kpSponsor := types.CreateKeyPair()

	adiSponsor := "RoadRunner"

	kpNewAdi := types.CreateKeyPair()
	routerAddress := fmt.Sprintf("tcp://localhost:%d", RandPort())

	//make a client, and also spin up the router grpc
	client, _, err := makeApiServiceClientAndServer(routerAddress)
	//r.AddBVCClient("network1", tmgrpc)
	if err != nil {
		t.Fatal(err)
	}

	jsonapi := API{RandPort(), validator.New(), client}
	//StartAPI(RandPort(), client)

	req := api.APIRequestRaw{}
	adi := &ADI{}
	adi.URL = "WileECoyote"
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
