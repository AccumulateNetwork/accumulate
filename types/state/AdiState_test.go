package state

import (
	"encoding/hex"
	"encoding/json"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"testing"
)

func TestIdentityState(t *testing.T) {
	var seed [32]byte

	_, err := hex.Decode(seed[:], []byte("36422e9560f56e0ead53a83b33aec9571d379291b5e292b88dec641a98ef05d8"))
	if err != nil {
		t.Fatal(err)
	}
	testidentity := "TestIdentity"

	key := ed25519.GenPrivKeyFromSecret(seed[:])

	ids := NewIdentityState(testidentity)
	err = ids.SetKeyData(KeyTypePublic, key.PubKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if ids.GetAdiChainPath() != "TestIdentity" {
		t.Fatalf("Invalid ADI stored in identity state, expected %s, received %s", ids.GetAdiChainPath(), testidentity)
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

	var id2 AdiState

	err = id2.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("Error unmarshalling binary %v", err)
	}

	if id2.GetAdiChainPath() != ids.GetAdiChainPath() {
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

	var id3 AdiState

	id2data, err := json.Marshal(&id2)
	if err != nil {
		t.Fatalf("Cannot marshal json")
	}
	err = json.Unmarshal(id2data, &id3)
	if err != nil {
		t.Fatalf("Cannot marshal json")
	}

}
