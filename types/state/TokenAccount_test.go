package state

import (
	"fmt"
	"math/big"
	"testing"
)

func TestTokenBalanceState(t *testing.T) {

	tokenUrl := "MyADI/MyTokenType"

	accountUrl := "MyADI/MyTokens"
	token := NewTokenAccount(accountUrl, tokenUrl)

	fmt.Println(token.GetBalance())

	deposit := big.NewInt(10000)

	err := token.AddBalance(deposit)
	if err != nil {
		t.Fatal(err)
	}

	expectedBalance := big.NewInt(10000)

	if token.GetBalance().Cmp(expectedBalance) != 0 {
		t.Fatalf("Token Balance Error %s", token.GetBalance())
	}

	withdrawal := big.NewInt(5000)

	expectedBalance.SetInt64(5000)

	err = token.SubBalance(withdrawal)
	if err != nil {
		t.Fatal(err)
	}
	if token.GetBalance().Cmp(expectedBalance) != 0 {
		t.Fatalf("Token Balance Error %s , expect 5000.00", token.GetBalance())
	}

	withdrawal.SetInt64(199)
	err = token.SubBalance(withdrawal)
	if err != nil {
		t.Fatal(err)
	}
	expectedBalance.SetInt64(4801)

	if token.GetBalance().Cmp(expectedBalance) != 0 {
		t.Fatalf("Token Balance Error %s , expect 4801", token.GetBalance())
	}

	withdrawal.SetInt64(4802) //insufficient balance
	//attempt to withdraw 4802.  We should get an exception now...
	err = token.SubBalance(withdrawal)
	//should fail with insufficient funds
	if err == nil {
		t.Fatal("Should have failed with negative balance")
	}

	fmt.Printf("Current balance %d\n", token.GetBalance())
	withdrawal.SetInt64(4801) //withdrawal it all.
	//attempt to withdraw 4802.  We should get an exception now...
	err = token.SubBalance(withdrawal)
	//should fail with insufficient funds
	if err != nil {
		t.Fatal("Account should have been emptied...")
	}

	expectedBalance.SetInt64(0)
	//balance should not have changed and still should be 4801,
	if token.GetBalance().Cmp(expectedBalance) != 0 {
		t.Fatalf("Cannot transact, token Balance %d , expected %d", token.GetBalance(), expectedBalance)
	}

	//Add 5001 to the balance
	deposit.SetInt64(5001)
	err = token.AddBalance(deposit)
	if err != nil {
		t.Fatal(err)
	}
	//since we emptied the account it should be 5001
	expectedBalance.SetInt64(5001)
	if token.GetBalance().Cmp(expectedBalance) != 0 {
		t.Fatalf("Expected a balance of %d, but got %d", expectedBalance, token.GetBalance())
	}

	tokenBytes, err := token.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	token2 := NewTokenAccount("blah", "blah")
	err = token2.UnmarshalBinary(tokenBytes)

	if err != nil {
		t.Fatal(err)
	}

	//account balance should still be 5001
	if token.GetBalance().Cmp(expectedBalance) != 0 {
		t.Fatalf("Unmarshal error, Expected a balance of %d, but got %d", expectedBalance, token.GetBalance())
	}
}
