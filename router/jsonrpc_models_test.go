package router

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/mitchellh/mapstructure"
	"testing"
	"time"

	//"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
)



func TestJsonRpcModels_Adi(t *testing.T) {

	kp := types.CreateKeyPair()


	var err error
	req := &APIRequest{}

	data := &ADI{}

	data.URL = "RedWagon"
	keyhash := sha256.Sum256(kp.PubKey().Bytes())
	copy(data.PublicKeyHash[:] , keyhash[:] )



	req.Tx = &APIRequestTx{}
	req.Tx.Data = data
	req.Tx.Timestamp = time.Now().Unix()
	req.Tx.Signer = &Signer{}
	req.Tx.Signer.URL = "redrock"
	copy(req.Tx.Signer.PublicKey[:], kp.PubKey().Bytes())

	params,err := json.Marshal(&req.Tx)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kp.Sign(params)

	reqraw := &APIRequestRaw{}

	reqraw.Tx = &json.RawMessage{}
	params = []byte("{\"hello\"     :   \"there how are    you\"}")
	*reqraw.Tx = params

	copy(reqraw.Sig[:], sig)

	params, err = json.Marshal(&reqraw)

	rawreq2 := APIRequestRaw{}
	err = json.Unmarshal(params, &rawreq2)

	validate := validator.New()

	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		t.Fatal(ErrorInvalidRequest)
	}


	// validate request
	if err = validate.Struct(req); err != nil {
		t.Fatal(err)
	}

	// parse req.tx.data
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = validate.Struct(data); err != nil {
		t.Fatal(err)
	}

	//resp := &TokenTx{}

}
