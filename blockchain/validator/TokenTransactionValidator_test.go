package validator

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"math/big"
	"testing"
	"time"
)

func CreateFakeIdentityState(identitychainpath string, key ed25519.PrivKey) (*state.Object, []byte) {
	id, _, _ := types.ParseIdentityChainPath(identitychainpath)

	idhash := sha256.Sum256([]byte(id))

	so := state.Object{}
	ids := state.NewIdentityState(id)
	ids.SetKeyData(0, key.PubKey().Bytes())
	so.Entry, _ = ids.MarshalBinary()

	eh := sha256.Sum256(so.Entry)
	so.EntryHash = eh[:]
	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, idhash[:]
}

func CreateFakeTokenAccountState(identitychainpath string, t *testing.T) *state.Object {
	_, cp, _ := types.ParseIdentityChainPath(identitychainpath)

	tokenPath := "wileecoyote/Acme"
	tas := state.NewTokenAccount(types.UrlChain(cp), types.UrlChain(tokenPath))

	deposit := big.NewInt(5000)
	tas.AddBalance(deposit)

	so := state.Object{}
	so.Entry, _ = tas.MarshalBinary()
	eh := sha256.Sum256(so.Entry)
	so.EntryHash = eh[:]
	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so
}

func CreateFakeTokenTransaction(t *testing.T, kp ed25519.PrivKey) *proto.Submission {
	tokenchainname := "RoadRunner/ACME"

	outputs := make(map[string]*big.Int)
	outputs["WileECoyote/MyACMEToken"] = big.NewInt(5000)

	tx := api.TokenTx{}
	amt := types.Amount{}
	amt.SetInt64(5000)
	tx.AddToAccount("WileECoyote/MyACMETokens", &amt)
	tx.From = types.UrlChain(tokenchainname)

	data, err := json.Marshal(&tx)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kp.Sign(data)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Data(data).
		ChainUrl(tokenchainname).
		Timestamp(time.Now().Unix()).
		PubKey(kp.PubKey().Bytes()).
		Signature(sig).Instruction(proto.AccInstruction_Token_Transaction).
		Build()

	if err != nil {
		t.Fatalf("Failed to make a token rpc call %v", err)
	}
	return sub
}

func TestTokenTransactionValidator_Check(t *testing.T) {
	kp := types.CreateKeyPair()
	identitychainpath := "RoadRunner/ACME"
	currentstate := StateEntry{}
	currentstate.ChainState = CreateFakeTokenAccountState(identitychainpath, t)
	var idhash []byte
	currentstate.IdentityState, idhash = CreateFakeIdentityState(identitychainpath, kp)

	chainhash := sha256.Sum256([]byte(identitychainpath))

	ttv := NewTokenTransactionValidator()

	faketx := CreateFakeTokenTransaction(t, kp)

	//need to simulate a state entry for chain and token
	err := ttv.Check(&currentstate, idhash, chainhash[:], 0, 0, faketx.Data)
	if err != nil {
		t.Fatalf("Error performing check %v", err)
	}
}
