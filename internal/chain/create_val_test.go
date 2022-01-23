package chain_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tmcrypto "github.com/tendermint/tendermint/crypto/ed25519"

	"github.com/AccumulateNetwork/accumulate/client"
	"github.com/AccumulateNetwork/accumulate/internal/chain"

	apiv2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
)


func TestCreateValidatorTx2(t *testing.T) {
	body := new(protocol.CreateValidator)
	body.Moniker = "Mohammedz Validator"
	body.Identity = "acc://Mohammed/Staling/Validator"
	body.Details = "Testing validator for mohamm"
	body.Website = "www.defidevs.io"
	body.ValidatorAddress = tmcrypto.GenPrivKey().PubKey().Address().String()
	body.PubKey = tmcrypto.GenPrivKey().PubKey().Bytes()


	data, err := json.Marshal(body)
	t.Log(err)
	t.Log(data)

	req := &apiv2.TxRequest{}


	raw := json.RawMessage(data)

	req.Payload = &raw
	req.Signer.PublicKey = body.PubKey
	req.Signer.Nonce = uint64(time.Now().Unix())

	var adiUrl, _ = url2.Parse("acc://Mohammed")
	req.Origin = adiUrl
	//req.KeyPage = &acmeapi.APIRequestKeyPage{}

	req.KeyPage.Height = 1
	copy(req.Signer.PublicKey[:], body.PubKey)
	t.Log(req)


	params, _ := json.Marshal(&req)
	c := client.NewAPIClient()
	c.Request(context.Background(), "create-validator", params, &body)
	t.Log(c)
	str, _ := json.Marshal(&body)

	t.Log(str)
	var st chain.StateManager
	st.Submit(adiUrl, body)

	//v := []*tmtype.Validator{}
	//st.AddValidator(v)

	t.Log(st)


}