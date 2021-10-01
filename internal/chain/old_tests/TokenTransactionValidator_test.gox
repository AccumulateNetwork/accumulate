package validator

import (
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
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

func CreateFakeTokenTransaction(t *testing.T, accountUrl string, kp ed25519.PrivKey) *transactions.GenTransaction {

	outputs := make(map[string]*big.Int)
	outputs["WileECoyote/MyACMEToken"] = big.NewInt(5000)

	tx := api.TokenTx{}
	amt := types.Amount{}
	amt.SetInt64(5000)
	adiChainPath := types.String("WileECoyote/MyACMETokens")
	tx.AddToAccount(adiChainPath, &amt)
	tx.From.String = types.String(accountUrl)

	data, _ := tx.MarshalBinary()
	sub := &transactions.GenTransaction{}

	//set the identity chain for the destination
	sub.Routing = types.GetAddressFromIdentity(adiChainPath.AsString())
	sub.ChainID = types.GetChainIdFromChainPath(adiChainPath.AsString()).Bytes()
	sub.SigInfo = &transactions.SignatureInfo{}
	sub.SigInfo.URL = *adiChainPath.AsString()
	sub.Transaction = data
	ed := new(transactions.ED25519Sig)
	ed.Sign(1, kp.Bytes(), sub.TransactionHash())

	sub.Signature = append([]*transactions.ED25519Sig{}, ed)

	return sub
}

func TestTokenTransactionValidator_Check(t *testing.T) {
	kp := types.CreateKeyPair()
	identitychainpath := "RoadRunner/ACME"
	currentstate := state.StateEntry{}
	currentstate.ChainState = CreateFakeTokenAccountState(identitychainpath, t)
	currentstate.IdentityState, _ = CreateFakeIdentityState(identitychainpath, kp)

	ttv := NewTokenTransactionValidator()

	faketx := CreateFakeTokenTransaction(t, "RoadRunner/ACME", kp)

	//need to simulate a state entry for chain and token
	err := ttv.Check(&currentstate, faketx)
	if err != nil {
		t.Fatalf("Error performing check %v", err)
	}
}
