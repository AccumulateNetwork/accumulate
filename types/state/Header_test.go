package state

import "testing"

func TestStateHeader(t *testing.T) {

	header := Header{"AIM-1", "acme/chain/path"}

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
