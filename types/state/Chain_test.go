package state

import (
	"bytes"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestStateHeader(t *testing.T) {

	header := Chain{ChainUrl: "acme/chain/path", Type: types.Bytes32(types.ChainTypeAnonTokenAccount)}

	data, err := header.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	header2 := Chain{}

	err = header2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Compare(header.GetType().Bytes(), header2.GetType().Bytes()) != 0 {
		t.Fatalf("header type doesnt match")
	}

	if header.GetChainUrl() != header2.GetChainUrl() {
		t.Fatalf("header adi chain path doesnt match")
	}

}
