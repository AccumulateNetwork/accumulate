package protocol

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types"
)

//NewTokenAccount create a new token account.  Requires the identity/chain id's and coinbase if applicable
func NewTokenAccountByUrls(accountUrl string, tokenUrl string) *TokenAccount {
	tas := NewTokenAccount()
	tas.ChainUrl = types.String(accountUrl)
	tas.TokenUrl = tokenUrl
	return tas
}

//Set will copy a token account state
func (app *TokenAccount) Set(accountState *TokenAccount) {
	if accountState == nil {
		return
	}
	app.Balance.Set(&accountState.Balance)
	app.ChainUrl = accountState.ChainUrl
	app.TokenUrl = accountState.TokenUrl
	app.Type = accountState.Type
}

// CanTransact returns true/false if there is a sufficient balance
func (app *TokenAccount) CanTransact(amt *big.Int) bool {
	//make sure the user has enough in the account to perform the transaction
	//if the balance is greater than or equal to the amount, then we are good.
	return app.GetBalance().Cmp(amt) >= 0
}

//SubBalance will subtract a balance form the account.  If this is a coinbase account,
//the balance will not be subtracted.
func (app *TokenAccount) SubBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("invalid input amount specified to subtract from balance")
	}

	if app.Balance.Cmp(amt) < 0 {
		return fmt.Errorf("insufficient balance, amount available : %d, requested, %d", app.GetBalance(), amt)
	}

	app.Balance.Sub(&app.Balance, amt)
	return nil
}

//GetBalance will return the balance of the account
func (app *TokenAccount) GetBalance() *big.Int {
	return &app.Balance
}

//AddBalance will add an amount to the balance. It only accepts a positive balance.
func (app *TokenAccount) AddBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("invalid input amount specified to add to balance")
	}

	if amt.Sign() <= 0 {
		return fmt.Errorf("amount to add to balance must be a positive amount")
	}

	app.Balance.Add(&app.Balance, amt)
	return nil
}

func (acct *TokenAccount) CreditTokens(amount *big.Int) bool {
	if amount == nil || amount.Sign() < 0 {
		return false
	}

	acct.Balance.Add(&acct.Balance, amount)
	return true
}

func (acct *TokenAccount) CanDebitTokens(amount *big.Int) bool {
	return amount != nil && acct.Balance.Cmp(amount) >= 0
}

func (acct *TokenAccount) DebitTokens(amount *big.Int) bool {
	if !acct.CanDebitTokens(amount) {
		return false
	}

	acct.Balance.Sub(&acct.Balance, amount)
	return true
}

func (acct *TokenAccount) NextTx() uint64 {
	return 0
}

func (acct *TokenAccount) ParseTokenUrl() (*url.URL, error) {
	return url.Parse(acct.TokenUrl)
}
