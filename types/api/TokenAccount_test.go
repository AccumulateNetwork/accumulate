package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestTokenAccount(t *testing.T) {
	tokenUrl := types.String("RoadRunner/BeepBeep")
	adiChainPath := types.String("WileECoyote/ACME")
	adi, chain, err := types.ParseIdentityChainPath(adiChainPath.AsString())
	if err != nil {
		t.Fatalf("%v", err)
	}

	account := NewTokenAccount(adiChainPath, tokenUrl)

	if *account.TokenURL.AsString() != chain {
		t.Fatalf("token Url didn't match")
	}

	if *account.URL.AsString() != adi {
		t.Fatalf("adi URL")
	}

	data, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s", string(data))

}
