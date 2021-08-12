package types

import (
	"math/big"
	"testing"
)

func TestTokenIssuance(t *testing.T) {
	supply := big.NewInt(50000)
	ti, err := NewTokenIssuance("FCT", supply, 1, TokenCirculationMode_Burn)
	if err != nil {
		t.Fatal(err)
	}

	//ti.Metadata.UnmarshalJSON([]byte("{\"hello\":\"there\"}"))

	data, err := ti.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	ti2 := TokenIssuance{}

	err = ti2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

}
