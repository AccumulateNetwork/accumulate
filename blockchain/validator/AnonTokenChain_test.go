package validator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	"github.com/martinlindhe/base36"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"strings"
	"testing"
	"time"
)

func CreateFakeSyntheticDeposit(t *testing.T, tokenUrl string, from string, to string, kp ed25519.PrivKey) *proto.Submission {
	deposit := synthetic.NewTokenTransactionDeposit()
	deposit.SourceChainId = *types.GetIdentityChainFromIdentity(from)
	deposit.SourceAdiChain = *types.GetChainIdFromChainPath(from)
	deposit.Txid = sha256.Sum256([]byte("generictxid"))
	deposit.DepositAmount.SetInt64(5000)
	deposit.SetTokenInfo(types.UrlChain(tokenUrl))

	data, err := json.Marshal(&deposit)
	if err != nil {
		return nil
	}

	ts := time.Now().Unix()
	ledger := types.MarshalBinaryLedgerChainId(deposit.SourceAdiChain.Bytes(), data, ts)
	sig, err := kp.Sign(ledger)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Data(data).
		AdiUrl(to).
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
	adi, _, _ := types.ParseIdentityChainPath(addressUrl)

	anonTokenChain := state.NewChain(types.UrlChain(adi), api.ChainTypeAnonTokenAccount[:])

	so := state.Object{}
	so.Entry, _ = anonTokenChain.MarshalBinary()
	eh := sha256.Sum256(so.Entry)
	so.EntryHash = eh[:]
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

func CreateFakeAnonTokenState(identitychainpath string, key ed25519.PrivKey) (*state.Object, []byte) {

	id, _, _ := types.ParseIdentityChainPath(identitychainpath)

	idhash := sha256.Sum256([]byte(id))

	so := state.Object{}
	ids := state.NewIdentityState(id)
	ids.SetKeyData(state.KeyTypeSha256, key.PubKey().Bytes())
	so.Entry, _ = ids.MarshalBinary()

	eh := sha256.Sum256(so.Entry)
	so.EntryHash = eh[:]
	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, idhash[:]
}

func TestAnonTokenChain_Validate(t *testing.T) {
	kp := types.CreateKeyPair()
	address := MakeAnonymousAddress(kp.PubKey().Bytes())
	anon := NewAnonTokenChain()
	tokenUrl := "roadrunner/MyAcmeTokens" //coinbase
	subDeposit := CreateFakeSyntheticDeposit(t, tokenUrl, tokenUrl, address, kp)
	stateObject := CreateFakeAnonymousTokenChain(address)

	anonAccountUrl := address + "/" + tokenUrl
	subTx := CreateFakeTokenTransaction(t, anonAccountUrl, kp)
	currentState := state.StateEntry{}
	stateDB := &state.StateDB{}
	stateDB.Open("/var/tmp", true, true)
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

	fmt.Printf("address = %s, chain spec %s\n", address, *anon.GetChainSpec())
	resp, err = anon.Validate(&currentState, subTx)

	if err != nil {
		t.Fatalf("fail for subTx, %v", err)
	}
	//now we can try the transaction again to see if it works now...

	//anon.Check()
}

func TestAddressGeneration(t *testing.T) {
	var pubkey types.Bytes32
	hex.Decode(pubkey[:], []byte("8301843BA7F7DE82901547EC1DC4F6505AB2BF078DB8F3460AE72D7B4250AD78"))

	keyhash := sha256.Sum256(pubkey[:])

	addr := "nothinghere"
	i := 0x000000
	helloWorld := sha256.Sum256([]byte("hello world"))
	fmt.Printf("%x\n", helloWorld)
	//strings.Contains
	//155e0001 9F5ACC sha256
	//9T e001 sha256d
	//5A 6e01 sha256d

	for strings.Contains(addr, "ACM3") == false {
		//prefix := fmt.Sprintf("%x", i)
		var prefix [4]byte
		hex.Decode(prefix[:], []byte(fmt.Sprintf("%x", i)))
		result := append(prefix[:], keyhash[:]...)
		hash1 := sha256.Sum256(result)
		hash2 := sha256.Sum256(hash1[:])
		encodeThis := append(result, hash2[:4]...)
		addr = base36.EncodeBytes(encodeThis)
		if i%1000 == 0 {
			fmt.Printf("%s %04x\n", addr, i)
		}

		addrb := []byte(addr)
		addr = string(addrb[:4])
		i++
		if i == 0xFFFF {
			break
		}
	}

	fmt.Printf("%s found at %x\n\n\n", addr, i)
	j := 0
	for j < 10000 {
		kp := types.CreateKeyPair()
		var prefix [4]byte
		hex.Decode(prefix[:], []byte(fmt.Sprintf("%x", i)))
		keyhash := kp.PubKey().Bytes()
		result := append(prefix[:], keyhash[:]...)
		hash1 := sha256.Sum256(result)
		hash2 := sha256.Sum256(hash1[:])
		encodeThis := append(result, hash2[:4]...)
		addr = base36.EncodeBytes(encodeThis)

		fmt.Printf("%s %04x\n", addr, j)

		j++
	}

}
