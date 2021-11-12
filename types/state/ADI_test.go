package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

func TestIdentityState(t *testing.T) {
	var seed [32]byte

	_, err := hex.Decode(seed[:], []byte("36422e9560f56e0ead53a83b33aec9571d379291b5e292b88dec641a98ef05d8"))
	if err != nil {
		t.Fatal(err)
	}
	testidentity := "TestIdentity"

	key := ed25519.GenPrivKeyFromSecret(seed[:])

	ids := NewIdentityState(types.String(testidentity))
	err = ids.SetKeyData(KeyTypePublic, key.PubKey().Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if ids.GetChainUrl() != "TestIdentity" {
		t.Fatalf("Invalid ADI stored in identity state, expected %s, received %s", ids.GetChainUrl(), testidentity)
	}

	ktype, keydata := ids.GetKeyData()

	if ktype != KeyTypePublic {
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

	if id2.GetChainUrl() != ids.GetChainUrl() {
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

func TestADI_VerifyKey(t *testing.T) {
	var seed [32]byte

	_, err := hex.Decode(seed[:], []byte("36422e9560f56e0ead53a83b33aec9571d379291b5e292b88dec641a98ef05d8"))
	require.NoError(t, err)
	key := ed25519.GenPrivKeyFromSecret(seed[:])
	pubKey := key.PubKey().Bytes()
	keyHash := sha256.Sum256(pubKey)
	keyHash2 := sha256.Sum256(keyHash[:])

	cases := []struct {
		Type KeyType
		Data []byte
	}{
		{KeyTypePublic, pubKey},
		{KeyTypeSha256, keyHash[:]},
		{KeyTypeSha256d, keyHash2[:]},
	}

	for _, a := range cases {
		for _, b := range cases {
			t.Run(fmt.Sprintf("%v x %v", a.Type, b.Type), func(t *testing.T) {
				if b.Type != KeyTypePublic {
					t.Skip("Not supported")
				}

				id := new(AdiState)
				id.KeyType = a.Type
				id.KeyData = a.Data
				require.True(t, id.VerifyKey(b.Data), "Verification failed")
			})
		}
	}
}
