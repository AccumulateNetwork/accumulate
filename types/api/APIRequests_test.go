package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func createAdiTx(adiUrl string, pubkey []byte) (string, error) {
	data := &ADI{}

	data.URL = types.String(adiUrl)
	keyhash := sha256.Sum256(pubkey)
	copy(data.PublicKeyHash[:], keyhash[:])

	ret, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(ret), nil
}

func createToken(tokenUrl string) (string, error) {
	data := &Token{}

	data.URL = types.String(tokenUrl)
	data.Precision = 8
	data.Symbol = "ACME"
	meta := json.RawMessage(fmt.Sprintf("{\"%s\":\"%s\"}", "test", "token"))
	data.Meta = &meta

	ret, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(ret), nil
}

func createRequest(t *testing.T, adiUrl string, kp *ed25519.PrivKey, message string) []byte {
	req := &APIRequestRaw{}

	req.Tx = &APIRequestRawTx{}
	// make a raw json message.
	raw := json.RawMessage(message)

	//Set the message data. Making it a json.RawMessage will prevent go from unmarshalling it which
	//allows us to verify the signature against it.
	req.Tx.Data = &raw
	req.Tx.Timestamp = time.Now().Unix()
	req.Tx.Signer = &Signer{}
	req.Tx.Signer.URL = types.String(adiUrl)
	copy(req.Tx.Signer.PublicKey[:], kp.PubKey().Bytes())

	//form the ledger for signing
	ledger := types.MarshalBinaryLedgerAdiChainPath(adiUrl, []byte(message), req.Tx.Timestamp)

	//sign it...
	sig, err := kp.Sign(ledger)

	//store the signature
	copy(req.Sig[:], sig)

	//make the json for submission to the jsonrpc
	params, err := json.Marshal(&req)
	if err != nil {
		t.Fatal(err)
	}

	return params
}

func TestAPIRequest_Adi(t *testing.T) {
	kp := types.CreateKeyPair()

	adiUrl := "redwagon"

	message, err := createAdiTx(adiUrl, kp.PubKey().Bytes())
	if err != nil {
		t.Fatal(err)

	}
	params := createRequest(t, adiUrl, &kp, message)

	validate := validator.New()

	req := &APIRequestRaw{}
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
	err = mapstructure.Decode(req.Tx.Data, data)
	if err == nil {
		//in this case we are EXPECTING failure because the mapstructure doesn't decode the hex encoded strings from data
		t.Fatal(err)
	}

	rawreq := APIRequestRaw{}
	err = json.Unmarshal(params, &rawreq)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(*rawreq.Tx.Data, data)
	if err != nil {
		t.Fatal(err)
	}

	// validate request data
	if err = validate.Struct(data); err != nil {
		//the data should have been unmarshalled correctly and the data is should be valid
		t.Fatal(err)
	}
}

func TestAPIRequest_Token(t *testing.T) {
	kp := types.CreateKeyPair()

	adiUrl := "redwagon"
	tokenUrl := adiUrl + "/MyTokens"

	message, err := createToken(tokenUrl)
	if err != nil {
		t.Fatal(err)
	}
	params := createRequest(t, adiUrl, &kp, message)

	validate := validator.New()

	req := &APIRequestRaw{}
	// unmarshal req
	if err = json.Unmarshal(params, &req); err != nil {
		t.Fatal(err)
	}

	// validate request
	if err = validate.Struct(req); err != nil {
		t.Fatal(err)
	}

	data := &Token{}

	// parse req.tx.data
	err = mapstructure.Decode(req.Tx.Data, data)
	if err == nil {
		//in this case we are EXPECTING failure because the mapstructure doesn't decode the hex encoded strings from data
		t.Fatal(err)
	}

	rawreq := APIRequestRaw{}
	err = json.Unmarshal(params, &rawreq)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(*rawreq.Tx.Data, data)
	if err != nil {
		t.Fatal(err)
	}

	// validate request data
	if err = validate.Struct(data); err != nil {
		//the data should have been unmarshalled correctly and the data is should be valid
		t.Fatal(err)
	}
}
