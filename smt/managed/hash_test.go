package managed_test

import (
	"bytes"
	"crypto/sha256"
	"testing"

	. "gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func TestHash(t *testing.T) {
	var h Hash
	var data = []byte("abc")
	hf := func(data []byte) Hash {
		return Sha256(data)
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
}
