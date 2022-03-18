package protocol

import (
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestED25519Sig(t *testing.T) {
	seed := sha256.Sum256([]byte{17, 26, 35, 44, 53, 62})
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	message := []byte("this is a message of some import")
	var timestamp uint64 = 1

	es1 := new(LegacyED25519Signature)
	es1.Timestamp = timestamp
	es1.PublicKey = append(es1.PublicKey, privateKey[32:]...)
	mh := sha256.Sum256(message)
	if err := es1.Sign(timestamp, privateKey, mh[:]); err != nil {
		t.Error("signature failed")
	}
	if !es1.Verify(mh[:]) {
		t.Error("verify signature message failed")
	}

	sigData, err := es1.MarshalBinary()
	if err != nil {
		t.Error(err)
	}
	es2 := new(LegacyED25519Signature)
	err = es2.UnmarshalBinary(sigData)
	if err != nil {
		t.Error(err)
	}
	if !es2.Verify(mh[:]) {
		t.Error("verify signature marshaled message failed")
	}
}
