package state

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTokenCoinbase(t *testing.T) {

	tokenUrl := "MyADI/MyTokenType"

	//issuedToken := api.NewToken(tokenUrl, "fct", 8)

	//accountUrl := "MyADI/MyTokens"
	account := NewToken(tokenUrl)

	jm := json.RawMessage{}
	jm = []byte("{\"test\":\"me\"}")
	account.Meta = &jm
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

	if bytes.Compare(account.GetType().Bytes(), account2.GetType().Bytes()) != 0 {
		t.Fatalf("chain types don't match")
	}

	if account.Precision != account2.Precision {
		t.Fatalf("precision doesn't match")
	}

	if account2.Meta == nil {
		t.Fatalf("meta is nil")
	}
	if bytes.Compare(*account2.Meta, jm) != 0 {
		t.Fatalf("meta data doesn't match")
	}
}
