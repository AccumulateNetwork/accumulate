package api

import (
	"testing"
)

func TestToken(t *testing.T) {

	token := NewToken("RoadRunner/ACME", "ACME", 8, "acc://RoadRunner/properties")

	data, err := token.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	ti2 := Token{}

	err = ti2.UnmarshalBinary(data)
	if err != nil {
		t.Fatal(err)
	}

	if ti2.URL != token.URL {
		t.Fatal("URL doesn't match")
	}
	if ti2.Precision != token.Precision {
		t.Fatal("Precision doesn't match")
	}
	if ti2.Symbol != token.Symbol {
		t.Fatal("Symbol doesn't match")
	}
	if ti2.PropertiesUrl != token.PropertiesUrl {
		t.Fatal("propertiesUrl doesn't match")
	}
}
