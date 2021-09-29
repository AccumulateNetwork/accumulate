package synthetic

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestAccStateSubmit(t *testing.T) {
	aState := AccStateSubmit{}
	aState.Height = 1234
	aState.NetworkId = 5
	aState.BptHash = sha256.Sum256([]byte("a bpt hash"))

	data, err := aState.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	bState := AccStateSubmit{}

	err = bState.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if aState.Height != bState.Height {
		t.Fatal("state height doesn't match")
	}

	if aState.NetworkId != bState.NetworkId {
		t.Fatal("state network id doesn't match")
	}

	if !bytes.Equal(aState.BptHash[:], bState.BptHash[:]) {
		t.Fatal("state bpt hash doesn't match")
	}
}
