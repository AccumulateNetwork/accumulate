package state

import (
	"bytes"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"testing"
)

func TestStateHeader(t *testing.T) {

	header := Chain{"acme/chain/path", types.Bytes32(api.ChainTypeAnonTokenAccount)}

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
