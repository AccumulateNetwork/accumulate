package validator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func CreateFakeSyntheticDeposit(t *testing.T, tokenUrl types.String, from types.String, to types.String, kp ed25519.PrivKey) *proto.Submission {
	txId := sha256.Sum256([]byte("Transaction Hash of Orig Tx"))
	deposit := synthetic.NewTokenTransactionDeposit(txId[:], &from, &to)
	_ = deposit.SetDeposit(&tokenUrl, big.NewInt(5000))

	data, err := json.Marshal(&deposit)
	if err != nil {
		return nil
	}

	ts := time.Now().Unix()
	chainId := types.GetChainIdFromChainPath(to.AsString())
	ledger := types.MarshalBinaryLedgerChainId(chainId[:], data, ts)
	sig, err := kp.Sign(ledger)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Data(data).
		AdiUrl(*to.AsString()).
		Timestamp(ts).
		PubKey(kp.PubKey().Bytes()).
		Signature(sig).Instruction(proto.AccInstruction_Synthetic_Token_Deposit).
		Build()

	if err != nil {
		t.Fatalf("Failed to make a token rpc call %v", err)
	}

	return sub
}

func CreateFakeAnonymousTokenChain(addressUrl string) *state.Object {
	adi, _, _ := types.ParseIdentityChainPath(&addressUrl)

	anonTokenChain := state.NewChain(types.String(adi), types.ChainTypeAnonTokenAccount[:])

	so := state.Object{}
	so.Entry, _ = anonTokenChain.MarshalBinary()

	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so
}
func MakeAnonymousAddress(key []byte) string {

	keyHash := sha256.Sum256(key)
	checkSum := sha256.Sum256(keyHash[:20])
	addrBytes := append(keyHash[:20], checkSum[:4]...)
	//generate the address from the key hash.
	address := fmt.Sprintf("0x%x", addrBytes)
	return address
}

func CreateFakeAnonTokenState(adiChainPath string, key ed25519.PrivKey) (*state.Object, []byte) {

	id, _, _ := types.ParseIdentityChainPath(&adiChainPath)

	idHash := sha256.Sum256([]byte(id))

	so := state.Object{}
	ids := state.NewIdentityState(id)
	_ = ids.SetKeyData(state.KeyTypeSha256, key.PubKey().Bytes())
	so.Entry, _ = ids.MarshalBinary()

	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, idHash[:]
}

func TestAnonTokenChain_BVC(t *testing.T) {
	appId := sha256.Sum256([]byte("anon"))

	bvc := NewBlockValidatorChain()
	kp := types.CreateKeyPair()
	address := MakeAnonymousAddress(kp.PubKey().Bytes())
	//anon := NewAnonTokenChain()
	tokenUrl := "roadrunner/MyAcmeTokens" //coinbase

	//todo: create fake deposit. or set an initial account state.
	//subDeposit := CreateFakeSyntheticDeposit(t, tokenUrl, tokenUrl, address, kp)
	//stateObject := CreateFakeAnonymousTokenChain(address)

	anonAccountUrl := address + "/" + tokenUrl
	subTx := CreateFakeTokenTransaction(t, anonAccountUrl, kp)

	stateDB := &state.StateDB{}
	err := stateDB.Open("/var/tmp", appId[:], true, true)
	if err != nil {
		t.Fatal(err)
	}

	se, err := state.NewStateEntry(nil, nil, stateDB)
	if err != nil {
		t.Fatal(err)
	}

	_, err = bvc.Validate(se, subTx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnonTokenChain_Validate(t *testing.T) {
	appId := sha256.Sum256([]byte("anon"))
	kp := types.CreateKeyPair()
	address := types.String(MakeAnonymousAddress(kp.PubKey().Bytes()))
	anon := NewAnonTokenChain()
	tokenUrl := types.String("roadrunner/MyAcmeTokens") //coinbase
	subDeposit := CreateFakeSyntheticDeposit(t, tokenUrl, tokenUrl, address, kp)
	stateObject := CreateFakeAnonymousTokenChain(*address.AsString())

	anonAccountUrl := address + "/" + tokenUrl
	subTx := CreateFakeTokenTransaction(t, *anonAccountUrl.AsString(), kp)
	currentState := state.StateEntry{}
	stateDB := &state.StateDB{}
	err := stateDB.Open("/var/tmp", appId[:], true, true)
	if err != nil {
		t.Fatal(err)
	}

	currentState.DB = stateDB
	currentState.IdentityState = stateObject
	resp, err := anon.Validate(&currentState, subTx)

	if err == nil {
		t.Fatalf("expecting an error on state object not existing")
	}
	resp, err = anon.Validate(&currentState, subDeposit)

	if len(resp.StateData) == 0 {
		t.Fatalf("expecting state changes to be returned")
	}

	if resp.Submissions != nil {
		t.Fatalf("not expecting submissions")
	}

	//store the returned states
	for k, v := range resp.StateData {
		err := stateDB.AddStateEntry(k[:], v)
		if err != nil {
			t.Fatalf("error storing state for chainId %x, %v", k, v)
		}
	}

	// fmt.Printf("address = %s, chain spec %s\n", address, *anon.GetChainSpec())
	resp, err = anon.Validate(&currentState, subTx)

	if err != nil {
		t.Fatalf("fail for subTx, %v", err)
	}
}
