package router

import (
	"github.com/tendermint/tendermint/crypto/ed25519"
	//"crypto/ed25519"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"math/big"
	"testing"
)

func CreateIdentityTest(identityname *string, key ed25519.PubKey, sponsor ed25519.PrivKey) (*proto.Submission, error) {
	sub := proto.Submission{}

	sub.Identitychain = types.GetIdentityChainFromAdi(*identityname).Bytes()
	sub.Chainid = types.GetIdentityChainFromAdi(*identityname).Bytes()
	sub.Type = 0 //this is going away it is not needed since we'll know the type from transaction
	sub.Instruction = proto.AccInstruction_Identity_Creation
	identitystate := types.IdentityState{}
	copy(identitystate.Publickey[:], key) //gomagic...
	identitystate.Adi = *identityname
	data, err := identitystate.MarshalBinary()
	if err != nil {
		fmt.Errorf("Error Marshalling Identity State %v", err)
		return nil, err
	}
	sub.Data = data
	sub.Signature = make([]byte, 64)

	sub.Key = make([]byte, 32)
	sig, err := sponsor.Sign(sub.Data)
	if err != nil {
		return nil, fmt.Errorf("Cannot sign data %v", err)
	}
	if sponsor.PubKey().VerifySignature(data, sig) == false {
		return nil, fmt.Errorf("Bad Signature")
	}
	copy(sub.Signature, sig)
	copy(sub.Key, sponsor.PubKey().Bytes())

	return &sub, nil
}

//This will create a submission message that for a token transaction.  Assume only 1 input and many outputs.
func CreateTokenTransactionTest(inputidentityname *string,
	intputchainname *string, inputamt *big.Int, outputs *map[string]*big.Int, metadata *string,
	signer ed25519.PrivKey) (*proto.Submission, error) {

	type AccTransaction struct {
		Input    map[string]*big.Int  `json:"inputs"`
		Output   *map[string]*big.Int `json:"outputs"`
		Metadata json.RawMessage      `json:"metadata,omitempty"`
	}

	var tx AccTransaction
	tx.Input = make(map[string]*big.Int)
	tx.Input[*intputchainname] = inputamt
	tx.Output = outputs
	if metadata != nil {
		tx.Metadata.UnmarshalJSON([]byte(fmt.Sprintf("{%s}", *metadata)))
	}

	txdata, err := json.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("Error formatting transaction, %v", err)
	}

	sig, err := signer.Sign(txdata)
	if err != nil {
		return nil, fmt.Errorf("Cannot sign data %v", err)
	}
	if signer.PubKey().VerifySignature(txdata, sig) == false {
		return nil, fmt.Errorf("Bad Signature")
	}

	sub := MakeSignedTokenTransactionBVCSubmission(*inputidentityname, *intputchainname, txdata, sig, signer.PubKey().(ed25519.PubKey))

	return sub, nil
}

func TestTokenTransfer(t *testing.T) {
	kp := CreateKeyPair()
	inputamt := big.NewInt(12345)

	identityname := "RedWagon"
	tokenchainname := "RedWagon/acc"

	outputs := make(map[string]*big.Int)
	outputs["RedRock/myacctoken"] = big.NewInt(12345)

	sub, err := CreateTokenTransactionTest(&identityname, &tokenchainname,
		inputamt, &outputs, nil, kp)
	if err != nil {
		t.Fatalf("Failed to make a token rpc call %v", err)
	}

	fmt.Println(string(sub.Data))

	if !json.Valid(sub.Data) {
		t.Fatal("Transaction test created invalid json")
	}

	if !kp.PubKey().VerifySignature(sub.Data, sub.Signature) {
		t.Fatal("Invalid signature for transaction")
	}
}

//func TestURLParser(t *testing.T) {
//
//	urlstring := "acc://RedWagon?key&prority=1"
//
//	q, err := URLParser(urlstring)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	fmt.Println(toJSON(q.Data))
//
//}

func TestURL(t *testing.T) {

	//the current scheme is a notional scheme.  word after ? indicates action to take i.e. the Submission instruction

	//create a URL with invalid utf8

	//create a URL without acc://

	//create a URL with sub account Wagon
	//Red is primary, and Wagon is secondary.
	urlstring := "acc://RedWagon/acc"
	q, err := URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	urlstring = "acc://RedWagon/acc?query&block=1000"
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	if string(q.Data) != "{\"block\":[\"1000\"]}" {
		t.Fatalf("URL query failed:  expected block=1000 received %s", string(q.Data))
	}

	urlstring = "acc://RedWagon/acc?currentblock&block=1000+index"
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	urlstring = "acc://RedWagon/identity?replace=PUBLICKEY1_HEX+PUBLICKEY2_HEX&nonce=54321&signature=f97a65de43&key=ba9356d0eabb3"
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	urlstring = "acc://RedWagon?replace=PUBLICKEY1_HEX+PUBLICKEY2_HEX&nonce=12345&signature=f97a65de43&key=ba9356d0eabb3"
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
}
