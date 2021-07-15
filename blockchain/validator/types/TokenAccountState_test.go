package types

import (
	"fmt"
	"testing"
)

func TestTokenBalanceState(t *testing.T) {

	token := TokenState{}

	fmt.Println(token.Balance())

	token.Credit("10000.00")
	if token.Balance() != "10000.00" {
		t.Fatalf("Token Balance Error %s",token.Balance())
	}

	token.Debit("5000.00")
	if token.Balance() != "5000.00" {
		t.Fatalf("Token Balance Error %s , expect 5000.00",token.Balance())
	}

	token.Debit("1.99")
	if token.Balance() != "4998.01" {
		t.Fatalf("Token Balance Error %s , expect 4998.01",token.Balance())
	}

	err := token.Debit("5000.00")
	//should fail with insufficient funds
	if err == nil {
		t.Fatal("Should have failed with negative balance")
	}

	if token.Balance() != "4998.01" {
		t.Fatalf("Token Balance Error %s , expect 4998.01",token.Balance())
	}

	err = token.Credit("1.00 FCT")
	//should fail with invalid input txt
	if err == nil {
		t.Fatal("Should have failed with invalid input txt")
	}

	err = token.Credit("1,00")
	//should fail with invalid input txt
	if err == nil {
		t.Fatal("Should have failed with invalid input txt")
	}

	fmt.Println()

	if token.Balance() != "5000" {
		t.Fatal("Should have failed with negative balance")
	}
}

func TestTokenTestMarshal(t *testing.T) {
	token := TokenState{}

	fmt.Println(token.Balance())

	token.Credit("10000.00")
	if token.Balance() != "10000.00" {
		t.Fatalf("Token Balance Error %s",token.Balance())
	}

	data, err := token.MarshalBinary()

	if err != nil {
		t.Fatal("Cannot marshal token structure")
	}

	token2 := TokenState{}

	err = token2.UnmarshalBinary(data)

	if err != nil {
		t.Fatal("Cannot unmarshal token structure")
	}

	fmt.Printf("Token (Premarshal) %s / Token (PostUnmarshal) %s\n", token.Balance(), token2.Balance())
	if token.Balance() != token2.Balance() {
		t.Fatalf("Balance of marshalled data does not equal that of unmarshalled data %s != %s", token.Balance(), token2.Balance())
	}
}
