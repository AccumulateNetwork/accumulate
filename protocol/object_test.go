package protocol

import (
	"testing"
)

func TestStateObject(t *testing.T) {
	var err error

	so := Object{}

	chain := new(AccountHeader)
	chain.Url = "myadi"
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
