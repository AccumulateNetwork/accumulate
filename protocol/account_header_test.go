package protocol

import (
	"testing"
)

func TestStateHeader(t *testing.T) {
	header := AccountHeader{Url: "acme/chain/path"}

	data, err := header.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	header2 := AccountHeader{}

	err = header2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if header.Url != header2.Url {
		t.Fatalf("header adi chain path doesnt match")
	}

}
