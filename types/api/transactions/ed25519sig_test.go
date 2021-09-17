package transactions

import (
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestED25519Sig(t *testing.T) {
	seed := sha256.Sum256([]byte{17, 26, 35, 44, 53, 62})
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	message := []byte("this is a message of some import")

	es1 := new(ED25519Sig)
	es1.PublicKey = append(es1.PublicKey, privateKey[32:]...)
	if err := es1.Sign(privateKey, message); err != nil {
		t.Error("signature failed")
	}
	if !es1.Verify(message) {
		t.Error("verify signature message failed")
	}

	hm := sha256.Sum256(message)
	if !es1.VerifyHash(hm[:]) {
		t.Error("verify signature hash failed")
	}
	sigData, err := es1.Marshal()
	if err != nil {
		t.Error(err)
	}
	es2 := new(ED25519Sig)
	es2.Unmarshal(sigData)
	if !es2.Verify(message) {
		t.Error("verify signature marshaled message failed")
	}
	hm = sha256.Sum256(message)
	if !es2.VerifyHash(hm[:]) {
		t.Error("verify signature marshaled hash failed")
	}
}
