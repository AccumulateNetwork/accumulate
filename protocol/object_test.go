package protocol

import (
	"testing"
)

func TestStateObject(t *testing.T) {
	var err error

	so := Object{}

	chain := new(AccountHeader)
	chain.Url = "myadi"
	chain.Type = AccountTypeLiteTokenAccount
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
