package validator

import (
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func CreateFakeIdentityState(identitychainpath string, key ed25519.PrivKey) (*state.Object, []byte) {
	id, _, _ := types.ParseIdentityChainPath(&identitychainpath)

	idhash := sha256.Sum256([]byte(id))

	so := state.Object{}
	ids := state.NewIdentityState(id)
	ids.SetKeyData(state.KeyTypeSha256, key.PubKey().Bytes())
	so.Entry, _ = ids.MarshalBinary()

	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so, idhash[:]
}

func CreateFakeTokenAccountState(identitychainpath string, t *testing.T) *state.Object {
	_, cp, _ := types.ParseIdentityChainPath(&identitychainpath)

	tokenPath := "wileecoyote/Acme"
	tas := state.NewTokenAccount(cp, tokenPath)

	deposit := big.NewInt(5000)
	tas.AddBalance(deposit)

	so := state.Object{}
	so.Entry, _ = tas.MarshalBinary()
	//we intentionally don't set the so.StateHash & so.PrevStateHash
	return &so
}

func CreateFakeTokenTransaction(t *testing.T, accountUrl string, kp ed25519.PrivKey) *proto.Submission {

	outputs := make(map[string]*big.Int)
	outputs["WileECoyote/MyACMEToken"] = big.NewInt(5000)

	tx := api.TokenTx{}
	amt := types.Amount{}
	amt.SetInt64(5000)
	adiChainPath := types.UrlChain{"WileECoyote/MyACMETokens"}
	tx.AddToAccount(adiChainPath, &amt)
	tx.From.String = types.String(accountUrl)

	data, err := json.Marshal(&tx)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Unix()
	ledger := types.MarshalBinaryLedgerChainId(types.GetIdentityChainFromIdentity(&accountUrl)[:], data, ts)
	sig, err := kp.Sign(ledger)
	if err != nil {
		t.Fatal(err)
	}

	builder := proto.SubmissionBuilder{}
	sub, err := builder.
		Data(data).
		ChainUrl(accountUrl).
		Timestamp(ts).
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
	currentstate := state.StateEntry{}
	currentstate.ChainState = CreateFakeTokenAccountState(identitychainpath, t)
	var idhash []byte
	currentstate.IdentityState, idhash = CreateFakeIdentityState(identitychainpath, kp)

	chainhash := sha256.Sum256([]byte(identitychainpath))

	ttv := NewTokenTransactionValidator()

	faketx := CreateFakeTokenTransaction(t, "RoadRunner/ACME", kp)

	//need to simulate a state entry for chain and token
	err := ttv.Check(&currentstate, idhash, chainhash[:], 0, 0, faketx.Data)
	if err != nil {
		t.Fatalf("Error performing check %v", err)
	}
}
