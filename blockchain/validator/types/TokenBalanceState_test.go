package types

import (
	"fmt"
	"testing"
)

func TestTokenBalanceState(t *testing.T) {

	token := TokenState{}

	token.Deposit("10000.00")
	if token.Balance() != "10000.00" {
		t.Fatalf("Token Balance Error %s",token.Balance())
	}

	token.Withdrawal("5000.00", "1.99")
	if token.Balance() != "4998.01" {
		t.Fatalf("Token Balance Error %s , expect 4998.01",token.Balance())
	}

	err := token.Withdrawal("5000.00", "1.99")
	//should fail with insufficient funds
	if err == nil {
		t.Fatal("Should have failed with negative balance")
	}

	if token.Balance() != "4998.01" {
		t.Fatalf("Token Balance Error %s , expect 4998.01",token.Balance())
	}

	err = token.Withdrawal("1.00 FCT", "1.99")
	//should fail with invalid input txt
	if err == nil {
		t.Fatal("Should have failed with invalid input txt")
	}


	err = token.Withdrawal("1.00", "abcdef")
	//should fail with invalid input txt
	if err == nil {
		t.Fatal("Should have failed with invalid fee txt")
	}

	fmt.Println(token.Balance())
}
