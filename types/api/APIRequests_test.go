package api

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	"testing"
	"time"
)

func createAdiTxJson(t *testing.T) []byte {
	kp := types.CreateKeyPair()

	var err error
	req := &APIRequest{}

	data := &ADI{}

	data.URL = "RedWagon"
	keyhash := sha256.Sum256(kp.PubKey().Bytes())
	copy(data.PublicKeyHash[:], keyhash[:])

	req.Tx = &APIRequestTx{}
	req.Tx.Data = data

	req.Tx.Timestamp = time.Now().Unix()
	req.Tx.Signer = &Signer{}
	req.Tx.Signer.URL = "redrock"
	copy(req.Tx.Signer.PublicKey[:], kp.PubKey().Bytes())

	params, err := json.Marshal(&req.Tx.Data)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kp.Sign(params)

	reqraw := &APIRequestRaw{}

	//reqraw.Tx = &json.RawMessage{}
	//*reqraw.Tx.Data = params

	copy(reqraw.Sig[:], sig)

	params, err = json.Marshal(&reqraw)
	if err != nil {
		t.Fatal(err)
	}

	return params
}

func TestAPIRequest_Adi(t *testing.T) {

	params := createAdiTxJson(t)

	rawreq2 := APIRequestRaw{}
	err := json.Unmarshal(params, &rawreq2)

	validate := validator.New()

	req := &APIRequest{}
	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		t.Fatal(err)
	}

	// validate request
	if err = validate.Struct(req); err != nil {
		t.Fatal(err)
	}

	data := &ADI{}

	// parse req.tx.data
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = validate.Struct(data); err != nil {
		t.Fatal(err)
	}

}
