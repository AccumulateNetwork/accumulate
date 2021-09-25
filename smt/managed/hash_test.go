package managed

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestHash(t *testing.T) {
	var h Hash
	var data = []byte("abc")
	hf := func(data []byte) Hash {
		return sha256.Sum256(data)
	}
	h = hf([]byte(data))
	h2 := h.Copy()
	if !bytes.Equal(h[:], h2[:]) {
		t.Error("copy failed")
	}
	h = h.Combine(hf, h2)
	h3 := sha256.Sum256(data)
	result := sha256.Sum256(append(h3[:], h3[:]...))
	if !bytes.Equal(h[:], result[:]) {
		t.Error("combine failed")
	}
	ph := h.CopyAndPoint()
	if !bytes.Equal(ph[:], h[:]) {
		t.Error("fail CopyAndPoint")
	}
	h[0] ^= 1
	if bytes.Equal(ph[:], h[:]) {
		t.Error("fail CopyAndPoint2")
	}
	if !bytes.Equal(h[:], h.Bytes()) {
		t.Error("fail Bytes()")
	}
}
