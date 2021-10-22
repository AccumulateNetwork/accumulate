package state

import (
	"testing"
)

func TestTokenCoinbase(t *testing.T) {

	tokenUrl := "MyADI/MyTokenType"

	//issuedToken := api.NewToken(tokenUrl, "fct", 8)

	//accountUrl := "MyADI/MyTokens"
	account := NewToken(tokenUrl)
	account.Precision = 12
	account.PropertiesUrl = "acc://MyADI/MyTokenProperties"
	actData, err := account.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	account2 := Token{}

	err = account2.UnmarshalBinary(actData)

	if err != nil {
		t.Fatal(err)
	}

	if account.Symbol != account2.Symbol {
		t.Fatalf("symbols do not match")
	}

	if account.GetChainUrl() != account2.GetChainUrl() {
		t.Fatalf("chain url's do not match")
	}

	if account.GetType() != account2.GetType() {
		t.Fatalf("chain types don't match")
	}

	if account.Precision != account2.Precision {
		t.Fatalf("precision doesn't match")
	}

	if account.PropertiesUrl != account2.PropertiesUrl {
		t.Fatalf("PropertiesUrl data doesn't match")
	}
}
