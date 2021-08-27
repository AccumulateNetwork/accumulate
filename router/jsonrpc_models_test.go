package router

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/mitchellh/mapstructure"

	//"github.com/mitchellh/mapstructure"
	"testing"
	"time"

	//"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/go-playground/validator/v10"
)

func createAdiTxJson(t *testing.T) []byte {
	kp := types.CreateKeyPair()

	var err error
	req := &api.APIRequestRaw{}

	data := &ADI{}

	data.URL = "RedWagon"
	keyhash := sha256.Sum256(kp.PubKey().Bytes())
	copy(data.PublicKeyHash[:], keyhash[:])

	req.Tx = &api.APIRequestRawTx{}
	req.Tx.Data = data
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

	reqraw.Tx = &json.RawMessage{}
	*reqraw.Tx = params

	copy(reqraw.Sig[:], sig)

	params, err = json.Marshal(&reqraw)
	if err != nil {
		t.Fatal(err)
	}

	return params
}

//submission builder
//SubmissionBuilder().PubKey().SenderUrl(),Tx().Signature(),Timestamp()
type SubmissionBuilder struct {
	sub proto.Submission
}

func (sb *SubmissionBuilder) Instruction(ins proto.AccInstruction) *SubmissionBuilder {
	sb.sub.Instruction = ins
	return sb
}

func (sb *SubmissionBuilder) Data(data []byte) *SubmissionBuilder {
	sb.sub.Data = data
	return sb
}

func (sb *SubmissionBuilder) Timestamp(timestamp int64) *SubmissionBuilder {
	sb.sub.Timestamp = timestamp
	return sb
}
func (sb *SubmissionBuilder) Signature(sig types.Bytes) *SubmissionBuilder {
	sb.sub.Signature = sig
	return sb
}
func (sb *SubmissionBuilder) PubKey(pubKey types.Bytes) *SubmissionBuilder {
	sb.sub.Key = pubKey[:]
	return sb
}

func (sb *SubmissionBuilder) ChainUrl(url string) *SubmissionBuilder {
	_, chain, _ := types.ParseIdentityChainPath(url)
	sb.sub.Chainid = types.GetChainIdFromChainPath(chain).Bytes()
	sb.sub.AdiChainPath = chain
	return sb
}

func (sb *SubmissionBuilder) AdiUrl(url string) *SubmissionBuilder {
	adi, _, _ := types.ParseIdentityChainPath(url)
	sb.sub.Identitychain = types.GetIdentityChainFromIdentity(adi).Bytes()
	if len(sb.sub.AdiChainPath) == 0 {
		sb.sub.AdiChainPath = adi
	}

	return sb
}

func (sb *SubmissionBuilder) Build() (*proto.Submission, error) {

	if sb.sub.Instruction == 0 {
		return nil, fmt.Errorf("instruction not set")
	}

	if len(sb.sub.Signature) != 64 {
		return nil, fmt.Errorf("invalid signature length")
	}

	if len(sb.sub.Data) == 0 {
		return nil, fmt.Errorf("no payload data set")
	}

	if len(sb.sub.AdiChainPath) == 0 {
		return nil, fmt.Errorf("invalid adi url")
	}

	if len(sb.sub.Key) != 32 {
		return nil, fmt.Errorf("invalid public key data length")
	}

	if sb.sub.Timestamp == 0 {
		return nil, fmt.Errorf("timestamp not set")
	}

	return &sb.sub, nil
}

func BuildSubmissionAdiCreate(raw *APIRequestRaw) (*proto.Submission, error) {
	req := &api.APIRequestRawTx{}
	err := json.Unmarshal(*raw.Tx, req)
	if err != nil {
		return nil, err
	}

	var subBuilder SubmissionBuilder
	sub, err := subBuilder.
		Instruction(proto.AccInstruction_Identity_Creation).
		Timestamp(req.Timestamp).
		Signature(raw.Sig[:]).
		AdiUrl(string(req.Signer.URL)).
		PubKey(req.Signer.PublicKey[:]).
		Data(*raw.Tx).
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

	if bytes.Compare(submission.Data, *rawreq2.Tx) != 0 {
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
