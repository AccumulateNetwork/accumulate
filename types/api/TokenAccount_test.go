package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/types"
)

func TestTokenAccount(t *testing.T) {
	tokenUrl := types.String("roadrunner/BeepBeep")
	adiChainPath := types.String("WileECoyote/ACME")
	_, chain, err := types.ParseIdentityChainPath(adiChainPath.AsString())
	if err != nil {
		t.Fatalf("%v", err)
	}

	account := NewTokenAccount(types.String(chain), tokenUrl)

	if account.TokenURL != tokenUrl {
		t.Fatalf("token Url didn't match")
	}

	if *account.URL.AsString() != chain {
		t.Fatalf("adi URL")
	}

	data, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s", string(data))

}
