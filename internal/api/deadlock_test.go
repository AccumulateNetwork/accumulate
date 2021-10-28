package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/response"
)

func TestDeadlock(t *testing.T) {
	//if runtime.GOOS == "windows" {
	//	t.Skip("Tendermint does not close all its open files on shutdown, which causes cleanup to fail")
	//}

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

	jsonapi := NewTest(t, query)

	res := jsonapi.Faucet(context.Background(), params)
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(data))

	//run the faucet again
	res = jsonapi.Faucet(context.Background(), params)
	data, err = json.Marshal(res)
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
