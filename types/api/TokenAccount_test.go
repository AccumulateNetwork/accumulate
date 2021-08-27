package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
)

func TestTokenAccount(t *testing.T) {
	account := NewTokenAccount("WileECoyote/ACME", "RoadRunner/BeepBeep")

	bal := big.NewInt(10000)
	accountBalance := NewTokenAccountWithBalance(account, bal)

	if accountBalance.Balance.Cmp(bal) != 0 {
		t.Fatalf("balance should be equal")
	}

	data, err := json.Marshal(&accountBalance)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s", string(data))

}
