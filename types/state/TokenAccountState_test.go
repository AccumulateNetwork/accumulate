package state

import (
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
	"testing"
)

func TestTokenBalanceState(t *testing.T) {

	token := NewTokenAccountState([]byte("issueid1"), []byte("issuechainid"), nil)

	fmt.Println(token.GetBalance())

	deposit := big.NewInt(10000)

	token.AddBalance(deposit)
	expected_balance := big.NewInt(10000)

	if token.GetBalance().Cmp(expected_balance) != 0 {
		t.Fatalf("Token Balance Error %s", token.GetBalance())
	}

	withdrawal := big.NewInt(5000)

	expected_balance.SetInt64(5000)

	token.SubBalance(withdrawal)
	if token.GetBalance().Cmp(expected_balance) != 0 {
		t.Fatalf("Token Balance Error %s , expect 5000.00", token.GetBalance())
	}

	withdrawal.SetInt64(199)
	token.SubBalance(withdrawal)
	expected_balance.SetInt64(4801)

	if token.GetBalance().Cmp(expected_balance) != 0 {
		t.Fatalf("Token Balance Error %s , expect 4801", token.GetBalance())
	}

	withdrawal.SetInt64(4802) //insufficient balance
	//attempt to withdraw 4802.  We should get an exception now...
	err := token.SubBalance(withdrawal)
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

	expected_balance.SetInt64(0)
	//balance should not have changed and still should be 4801,
	if token.GetBalance().Cmp(expected_balance) != 0 {
		t.Fatalf("Cannot transact, token Balance %d , expected %d", token.GetBalance(), expected_balance)
	}

	//Add 5001 to the balance
	deposit.SetInt64(5001)
	err = token.AddBalance(deposit)
	//since we emptied the account it should be 5001
	expected_balance.SetInt64(5001)
	if token.GetBalance().Cmp(expected_balance) != 0 {
		t.Fatalf("Expected a balance of %d, but got %d", expected_balance, token.GetBalance())
	}

	tokenbytes, err := token.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	token2 := NewTokenAccountState([]byte("blah"), []byte("blah"), nil)
	err = token2.UnmarshalBinary(tokenbytes)

	if err != nil {
		t.Fatal(err)
	}

	//account balance should still be 5001
	if token.GetBalance().Cmp(expected_balance) != 0 {
		t.Fatalf("Unmarshal error, Expected a balance of %d, but got %d", expected_balance, token.GetBalance())
	}
}

func TestTokenCoinbase(t *testing.T) {
	supply := big.NewInt(200000000000000)
	issuedtoken, err := types.NewTokenIssuance("FCT", supply, 8, types.TokenCirculationMode_Burn_and_Mint)

	if err != nil {
		t.Fatal(err)
	}

	identityhash := sha256.Sum256([]byte("MyADI"))
	chainid := sha256.Sum256([]byte("MyADI/MyTokenType"))
	account := NewTokenAccountState(identityhash[:], chainid[:], issuedtoken)

	actdata, err := account.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	account2 := TokenAccountState{}

	err = account2.UnmarshalBinary(actdata)

	if err != nil {
		t.Fatal(err)
	}

	if account.IsCoinbaseAccount() == false {
		t.Fatal("Should be a coinbase account")
	}

	amt := big.NewInt(100000000)

	account.SubBalance(amt)

	balancetest := big.NewInt(199999900000000)

	if account.GetBalance().Cmp(balancetest) != 0 {
		t.Fatalf("SubBalance fail: Balance should be %s, but is %s", balancetest.String(), account2.GetBalance().String())
	}

	account2.AddBalance(amt)

	if account2.GetBalance().Cmp(supply) == 0 {
		t.Fatalf("Burn and Mint AddBalance fail: Balance should be %s, but is %s", balancetest.String(), account2.GetBalance().String())
	}

	//setup burn only tokens
	coinbase2, err := types.NewTokenIssuance("ATK", supply, 8, types.TokenCirculationMode_Burn)
	account3 := NewTokenAccountState(identityhash[:], chainid[:], coinbase2)

	account3.SubBalance(amt)

	//check to make sure balance was subtracted
	if account3.GetBalance().Cmp(balancetest) != 0 {
		t.Fatalf("SubBalance fail: Balance should be %s, but is %s", balancetest.String(), account2.GetBalance().String())
	}

	account3.AddBalance(amt)

	//check to make sure tokens were not added back, i.e tokens are burned
	if account3.GetBalance().Cmp(balancetest) != 0 {
		t.Fatalf("Burn Test AddBalance fail: Balance should be %s, but is %s", balancetest.String(), account2.GetBalance().String())
	}
}
