package types

import (
	"encoding/hex"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"testing"
)

func TestIdentityState(t *testing.T) {
	var seed [32]byte
	hex.Decode(seed[:], []byte("36422e9560f56e0ead53a83b33aec9571d379291b5e292b88dec641a98ef05d8"))

	testidentity := "TestIdentity"

	key := ed25519.GenPrivKeyFromSecret(seed[:])

	ids := NewIdentityState(testidentity)
	ids.SetKeyData(0, key.PubKey().Bytes())

	if ids.GetAdi() != "TestIdentity" {
		t.Fatalf("Invalid ADI stored in identity state, expected %s, received %s", ids.GetAdi(), testidentity)
	}

	ktype, keydata := ids.GetKeyData()

	if ktype != 0 {
		t.Fatalf("Invalid Key Data Type")
	}

	if keydata == nil {
		t.Fatalf("Key data is null")
	}
	var testkey ed25519.PubKey

	copy(testkey.Bytes(), keydata)

	if testkey.Equals(key.PubKey()) {
		t.Fatalf("key's do not match")
	}

	data, err := ids.MarshalBinary()
	if err != nil {
		t.Fatalf("Error marshalling binary %v", err)
	}

	var id2 IdentityState

	err = id2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("Error unmarshalling binary %v", err)
	}

	if id2.GetAdi() != ids.GetAdi() {
		t.Fatalf("Adi's do not match after unmarshalling")
	}

	ktype2, keydata2 := id2.GetKeyData()
	var testkey2 ed25519.PubKey

	copy(testkey2.Bytes(), keydata2)

	if !testkey2.Equals(testkey) {
		t.Fatalf("Key's do not match after unmarshalling")
	}

	if ktype2 != ktype {
		t.Fatalf("Key's do not match after unmarshalling")
	}

}

func TestIdentityState_GetIdentityChainId(t *testing.T) {

}

func TestIdentityState_GetPublicKey(t *testing.T) {

}
