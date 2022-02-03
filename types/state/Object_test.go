package state

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/types"
)

func TestStateObject(t *testing.T) {
	var err error

	so := Object{}

	adi := types.String("myadi")
	chain := new(ChainHeader)
	chain.SetHeader(adi, types.AccountTypeLiteTokenAccount)
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
