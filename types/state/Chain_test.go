package state

import (
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestStateHeader(t *testing.T) {

	header := ChainHeader{ChainUrl: "acme/chain/path", Type: types.ChainTypeAnonTokenAccount}

	data, err := header.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	header2 := ChainHeader{}

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
