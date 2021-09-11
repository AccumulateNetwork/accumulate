package state

import (
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
)

func TestStateObject(t *testing.T) {

	var err error

	so := Object{}

	so.StateIndex = 1234

	adi := types.String("myadi")
	chain := NewChain(adi, api.ChainTypeAnonTokenAccount[:])
	so.Entry, err = chain.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	data, err := so.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	so2 := Object{}

	err = so2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

}
