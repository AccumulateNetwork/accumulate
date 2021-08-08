package types

import (
	"crypto/sha256"
	"github.com/Factom-Asset-Tokens/factom/fat"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"time"

	//"crypto/ed25519"
	"encoding/json"
	"fmt"
	//"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"math/big"
	"testing"
)

//
//func CreateIdentityTest(identityname *string, key ed25519.PubKey, sponsor ed25519.PrivKey) (*proto.Submission, error) {
//	sub := proto.Submission{}
//
//	sub.Identitychain = types.GetIdentityChainFromAdi(*identityname).Bytes()
//	sub.Chainid = types.GetIdentityChainFromAdi(*identityname).Bytes()
//	sub.Type = 0 //this is going away it is not needed since we'll know the type from transaction
//	sub.Instruction = proto.AccInstruction_Identity_Creation
//	identitystate := types.NewIdentityState(*identityname)
//	identitystate.SetKeyData(0,key)
//
//	data, err := identitystate.MarshalBinary()
//	if err != nil {
//		fmt.Errorf("Error Marshalling Identity State %v", err)
//		return nil, err
//	}
//	sub.Data = data
//	sub.Signature = make([]byte, 64)
//
//	sub.Key = make([]byte, 32)
//	sig, err := sponsor.Sign(sub.Data)
//	if err != nil {
//		return nil, fmt.Errorf("Cannot sign data %v", err)
//	}
//	if sponsor.PubKey().VerifySignature(data, sig) == false {
//		return nil, fmt.Errorf("Bad Signature")
//	}
//	copy(sub.Signature, sig)
//	copy(sub.Key, sponsor.PubKey().Bytes())
//
//	return &sub, nil
//}

func TestTokenTransfer(t *testing.T) {
	kp := CreateKeyPair()
	inputamt := big.NewInt(12345)

	identityname := "RedWagon"
	tokenchainname := "RedWagon/acc"

	outputs := make(map[string]*big.Int)
	outputs["RedRock/myacctoken"] = big.NewInt(12345)

	sub, err := CreateTokenTransaction(&identityname, &tokenchainname,
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

func MakeUpdateKeyURL(identityname string, oldkey ed25519.PrivKey, newkey ed25519.PubKey) string {

	kp1hash := sha256.Sum256(oldkey.PubKey().Bytes())
	kp2hash := sha256.Sum256(newkey.Bytes())

	payload := fmt.Sprintf("{ \"curkeyhash\": \"%x\", \"newkeyhash\": \"%x\" }", kp1hash[:], kp2hash[:])

	instruction := "ku"
	timestamp := time.Now().Unix()

	//build the message to be signed ed25519( sha256(identityname) | sha256(raw payload) | timestamp )
	msg := MarshalBinaryLedgerAdiChainPath(identityname, []byte(payload), timestamp)

	sig, _ := oldkey.Sign(msg)

	///update identity key
	urlstring := BuildAccumulateURL(identityname, instruction, []byte(payload), timestamp, oldkey.PubKey().Bytes(), sig)

	return urlstring
}

func MakeCreateIdentityURL(identityname string, sponsoridentityname string, sponsorkey ed25519.PrivKey, key ed25519.PubKey) string {
	kp2hash := sha256.Sum256(key.Bytes())

	payload := fmt.Sprintf("{ \"sponsor-identity\": \"%s\", \"initial-key-hash\": \"%x\" }", sponsoridentityname, kp2hash[:])

	instruction := "identity-create"
	timestamp := time.Now().Unix()

	msg := MarshalBinaryLedgerAdiChainPath(identityname, []byte(payload), timestamp)
	sig, _ := sponsorkey.Sign(msg)

	///create identity
	urlstring := BuildAccumulateURL(identityname, instruction, []byte(payload), timestamp, sponsorkey.PubKey().Bytes(), sig)

	return urlstring
}

func BuildAccumulateURL(fullchainpath string, ins string, payload []byte, timestamp int64, key []byte, sig []byte) string {
	return fmt.Sprintf("acc://%s?%s&payload=%x&timestamp=%d&key=%x&sig=%x", fullchainpath, ins, payload, timestamp, key, sig)
}

func MakeTokenIssueURL(fullchainpath string, supply int64, precision uint, symbol string, issuerkey ed25519.PrivKey) string {
	tx := fat.Issuance{}
	tx.Type = fat.TypeFAT0
	tx.Supply = supply
	tx.Precision = precision
	tx.Symbol = symbol
	payload, err := json.Marshal(tx)
	if err != nil {
		return ""
	}

	instruction := "token-issue"
	timestamp := time.Now().Unix()
	msg := MarshalBinaryLedgerAdiChainPath(fullchainpath, []byte(payload), timestamp)
	sig, _ := issuerkey.Sign(msg)

	urlstring := BuildAccumulateURL(fullchainpath, instruction, []byte(payload), timestamp, issuerkey.PubKey().Bytes(), sig)

	return urlstring
}
func MakeTokenTransactionURL(intputfullchainpath string, inputamt *big.Int, outputs *map[string]*big.Int, metadata string,
	signer ed25519.PrivKey) (string, error) {

	type AccTransaction struct {
		Input    map[string]*big.Int  `json:"inputs"`
		Output   *map[string]*big.Int `json:"outputs"`
		Metadata json.RawMessage      `json:"metadata,omitempty"`
	}

	var tx AccTransaction
	tx.Input = make(map[string]*big.Int)
	tx.Input[intputfullchainpath] = inputamt
	tx.Output = outputs
	if metadata != "" {
		tx.Metadata.UnmarshalJSON([]byte(fmt.Sprintf("{%s}", metadata)))
	}

	payload, err := json.Marshal(tx)
	if err != nil {
		return "", fmt.Errorf("Error formatting transaction, %v", err)
	}

	timestamp := time.Now().Unix()
	msg := MarshalBinaryLedgerAdiChainPath(intputfullchainpath, payload, timestamp)
	sig, err := signer.Sign(msg)
	if err != nil {
		return "", fmt.Errorf("Cannot sign data %v", err)
	}

	urlstring := BuildAccumulateURL(intputfullchainpath, "tx", payload, timestamp, signer.PubKey().(ed25519.PubKey), sig)

	return urlstring, nil
}

func TestURL(t *testing.T) {

	//create a keypair to use...

	//the current scheme is a notional scheme.  word after ? indicates action to take i.e. the Submission instruction

	//create a URL with invalid utf8

	//create a URL without acc://

	params := Subtx{}

	//Test identity name and chain path
	//identity name should be RedWagon and chainpath should be RedWagon/acc
	urlstring := "acc://RedWagon/acc"
	q, err := URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
	params.Set("RedWagon", q)
	result, _ := params.MarshalJSON()
	fmt.Println(string(result))

	urlstring = "acc://RedWagon/acc?query&block=1000"
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
	params.Set("RedWagon/acc", q)
	result, _ = params.MarshalJSON()
	fmt.Println(string(result))

	if string(q.Data) != "{\"block\":[\"1000\"]}" {
		t.Fatalf("URL query failed:  expected block=1000 received %s", string(q.Data))
	}

	urlstring = "acc://RedWagon/acc?query&block=1000+index"
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
	params.Set("RedWagon/acc", q)
	result, _ = params.MarshalJSON()
	fmt.Println(string(result))

	identityname := "RedWagon"

	kp1 := CreateKeyPair()

	kp2 := CreateKeyPair()
	sponsorname := "GreenRock"
	urlstring = MakeCreateIdentityURL(identityname, sponsorname, kp1, kp2.PubKey().(ed25519.PubKey))
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
	params.Set(identityname, q)
	result, _ = params.MarshalJSON()
	fmt.Println(string(result))

	urlstring = MakeUpdateKeyURL(identityname, kp1, kp2.PubKey().(ed25519.PubKey))
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
	params.Set(identityname, q)
	result, _ = params.MarshalJSON()
	fmt.Println(string(result))

	chainpath := identityname + "/" + "ATKCoinbase"
	urlstring = MakeTokenIssueURL(chainpath, 500000000, 8, "ATK", kp1)
	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
	params.Set(chainpath, q)
	result, _ = params.MarshalJSON()
	fmt.Println(string(result))

	chainpath = identityname + "/" + "MyAtkTokens"

	inpamt := big.NewInt(10000)
	outamt := big.NewInt(10000)
	outchainpath := "GreenRock/YourAtkTokens"
	out := make(map[string]*big.Int)
	out[outchainpath] = outamt

	urlstring, err = MakeTokenTransactionURL(chainpath, inpamt, &out, string(""), kp1)
	if err != nil {
		t.Fatalf("Error creating token transaction %v", err)
	}

	q, err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	params.Set(chainpath, q)
	result, _ = params.MarshalJSON()
	fmt.Println(string(result))
	//the q objects can be submitted to the router for processing.
}
