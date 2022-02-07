package protocol

import (
	"testing"
)

func TestStateHeader(t *testing.T) {

	header := AccountHeader{Url: "acme/chain/path", Type: AccountTypeLiteTokenAccount}

	data, err := header.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	header2 := AccountHeader{}

	err = header2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if header.GetType() != header2.GetType() {
		t.Fatalf("header type doesnt match")
	}

	if header.GetChainUrl() != header2.GetChainUrl() {
		t.Fatalf("header adi chain path doesnt match")
	}

}
