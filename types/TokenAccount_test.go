package types

import (
	"math/big"
	"testing"
)

func TestTokenAccount(t *testing.T) {
	account := NewTokenAccount("WileECoyote/ACME", "RoadRunner/BeepBeep")

	bal := big.NewInt(10000)
	accountbalance := NewTokenAccountWithBalance(account, bal)

	if accountbalance.Balance.Cmp(bal) != 0 {
		t.Fatalf("balance should be equal")
	}
}
