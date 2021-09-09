package refactor

import (
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestED25519Sig(t *testing.T) {
	seed := sha256.Sum256([]byte{17, 26, 35, 44, 53, 62})
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	message := []byte("this is a message of some import")
	es := Sign(privateKey, message)
	if !es.Verify(message) {
		t.Error("signature failed")
	}
	hm := sha256.Sum256(message)
	if !es.VerifyHash(hm[:]) {
		t.Error("signature hash failed")
	}
	sigData := es.Marshal()
	es2 := new(ED25519Sig)
	es2.Unmarshal(sigData)
	if !es2.Verify(message) {
		t.Error("signature failed")
	}
	hm = sha256.Sum256(message)
	if !es2.VerifyHash(hm[:]) {
		t.Error("signature hash failed")
	}
}
