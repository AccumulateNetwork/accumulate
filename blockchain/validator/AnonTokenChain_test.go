package validator

import (
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"testing"
)

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
	//anon.Check()
	fmt.Printf("address = %s, chain spec %s\n", address, *anon.GetChainSpec())
	//anon.Check()
}
