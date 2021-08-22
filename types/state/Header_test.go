package state

import (
	"crypto/sha256"
	"testing"
)

func TestStateHeader(t *testing.T) {

	aimHash := sha256.Sum256([]byte("AIM/1/0.1"))
	header := Header{aimHash, "acme/chain/path"}

	data, err := header.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	header2 := Header{}

	err = header2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if header.GetType() != header2.GetType() {
		t.Fatalf("header type doesnt match")
	}

	if header.GetAdiChainPath() != header2.GetAdiChainPath() {
		t.Fatalf("header adi chain path doesnt match")
	}

}
