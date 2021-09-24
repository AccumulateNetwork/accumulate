package router

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
)

func createAdiTxJson(t *testing.T) []byte {
	kp := types.CreateKeyPair()

	var err error
	req := &api.APIRequestRaw{}

	data := &ADI{}

	data.URL = "RedWagon"
	keyhash := sha256.Sum256(kp.PubKey().Bytes())
	copy(data.PublicKeyHash[:], keyhash[:])

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	raw := json.RawMessage{}
	raw = jsonData
	req.Tx = &api.APIRequestRawTx{}
	req.Tx.Data = &raw
	req.Tx.Timestamp = time.Now().Unix()
	req.Tx.Signer = &api.Signer{}
	req.Tx.Signer.URL = "redrock"
	copy(req.Tx.Signer.PublicKey[:], kp.PubKey().Bytes())

	params, err := json.Marshal(&req.Tx)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kp.Sign(params)

	reqraw := &api.APIRequestRaw{}

	rm := &api.APIRequestRawTx{}
	rm.Data = &json.RawMessage{}
	*rm.Data = params
	rm.Signer = req.Tx.Signer
	reqraw.Tx = rm

	copy(reqraw.Sig[:], sig)

	params, err = json.Marshal(&reqraw)
	if err != nil {
		t.Fatal(err)
	}

	return params
}

func BuildSubmissionAdiCreate(raw *api.APIRequestRaw) (*proto.Submission, error) {
	req := &api.APIRequestRawTx{}

	err := json.Unmarshal(*raw.Tx.Data, req)
	if err != nil {
		return nil, err
	}

	var subBuilder proto.SubmissionBuilder
	sub, err := subBuilder.
		Instruction(proto.AccInstruction_Identity_Creation).
		Timestamp(req.Timestamp).
		Signature(raw.Sig[:]).
		AdiUrl(string(req.Signer.URL)).
		PubKey(req.Signer.PublicKey[:]).
		Data(*raw.Tx.Data).
		Build()

	if err != nil {
		return nil, err
	}

	return sub, nil
}

func TestJsonRpcModels_Adi(t *testing.T) {
	//myjson := fmt.Sprintf("{\"%s\"    :  \"%s\" }", "hello", "world")

	params := createAdiTxJson(t)

	rawreq2 := api.APIRequestRaw{}
	err := json.Unmarshal(params, &rawreq2)

	submission, err := BuildSubmissionAdiCreate(&rawreq2)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(submission.Data, *rawreq2.Tx.Data) != 0 {
		t.Fatalf("submission data doesn't match transaction data")
	}

	validate := validator.New()

	req := &api.APIRequestRaw{}

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
	//err = json.Unmarshal(req.Tx.Data, data)
	mapstructure.Decode(req.Tx.Data, data)

	// validate request data
	if err = validate.Struct(data); err != nil {
		t.Fatal(err)
	}

	//resp := &TokenTx{}

}
