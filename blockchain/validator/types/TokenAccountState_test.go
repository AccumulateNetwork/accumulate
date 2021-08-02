package types

import (
	"fmt"
	"math/big"
	"testing"
)

func TestTokenBalanceState(t *testing.T) {

	token := TokenAccountState{}

	fmt.Println(token.Balance())

	deposit := big.NewInt(10000)

	token.AddBalance(deposit)
	expected_balance := big.NewInt(10000)

	if token.Balance().Cmp(expected_balance) != 0 {
		t.Fatalf("Token Balance Error %s", token.Balance())
	}

	withdrawal := big.NewInt(5000)

	expected_balance.SetInt64(5000)

	token.SubBalance(withdrawal)
	if token.Balance().Cmp(expected_balance) != 0 {
		t.Fatalf("Token Balance Error %s , expect 5000.00", token.Balance())
	}

	withdrawal.SetInt64(199)
	token.SubBalance(withdrawal)
	expected_balance.SetInt64(4801)

	if token.Balance().Cmp(expected_balance) != 0 {
		t.Fatalf("Token Balance Error %s , expect 4801", token.Balance())
	}

	withdrawal.SetInt64(4802) //insufficient balance
	//attempt to withdraw 4802.  We should get an exception now...
	err := token.SubBalance(withdrawal)
	//should fail with insufficient funds
	if err == nil {
		t.Fatal("Should have failed with negative balance")
	}

	fmt.Printf("Current balance %d\n", token.Balance())
	withdrawal.SetInt64(4801) //withdrawal it all.
	//attempt to withdraw 4802.  We should get an exception now...
	err = token.SubBalance(withdrawal)
	//should fail with insufficient funds
	if err != nil {
		t.Fatal("Account should have been emptied...")
	}

	expected_balance.SetInt64(0)
	//balance should not have changed and still should be 4801,
	if token.Balance().Cmp(expected_balance) != 0 {
		t.Fatalf("Cannot transact, token Balance %d , expected %d", token.Balance(), expected_balance)
	}

	//Add 5001 to the balance
	deposit.SetInt64(5001)
	err = token.AddBalance(deposit)
	//since we emptied the account it should be 5001
	expected_balance.SetInt64(5001)
	if token.Balance().Cmp(expected_balance) != 0 {
		t.Fatalf("Expected a balance of %d, but got %d", expected_balance, token.Balance())
	}

	tokenbytes, err := token.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	err = token.UnmarshalBinary(tokenbytes)

	if err != nil {
		t.Fatal(err)
	}

	//account balance should still be 5001
	if token.Balance().Cmp(expected_balance) != 0 {
		t.Fatalf("Unmarshal error, Expected a balance of %d, but got %d", expected_balance, token.Balance())
	}

}
