package protocol

import (
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
)

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

func (acct *TokenAccount) ParseTokenUrl() (*url.URL, error) {
	return url.Parse(acct.TokenUrl)
}

func (ms *KeyPage) CreditCredits(amount uint64) {
	amt := new(big.Int)
	amt.SetUint64(amount)
	ms.CreditBalance.Add(&ms.CreditBalance, amt)
}

func (ms *KeyPage) DebitCredits(amount uint64) bool {
	amt := new(big.Int)
	amt.SetUint64(amount)
	if amt.Cmp(&ms.CreditBalance) > 0 {
		return false
	}

	ms.CreditBalance.Sub(&ms.CreditBalance, amt)
	return true
}

func (acct *LiteTokenAccount) CreditTokens(amount *big.Int) bool {
	if amount == nil || amount.Sign() < 0 {
		return false
	}

	acct.Balance.Add(&acct.Balance, amount)
	return true
}

func (acct *LiteTokenAccount) CanDebitTokens(amount *big.Int) bool {
	return amount != nil && acct.Balance.Cmp(amount) >= 0
}

func (acct *LiteTokenAccount) DebitTokens(amount *big.Int) bool {
	if !acct.CanDebitTokens(amount) {
		return false
	}

	acct.Balance.Sub(&acct.Balance, amount)
	return true
}

func (acct *LiteTokenAccount) CreditCredits(amount uint64) {
	amt := new(big.Int)
	amt.SetUint64(amount)
	acct.CreditBalance.Add(&acct.CreditBalance, amt)
}

func (acct *LiteTokenAccount) DebitCredits(amount uint64) bool {
	amt := new(big.Int)
	amt.SetUint64(amount)
	if amt.Cmp(&acct.CreditBalance) > 0 {
		return false
	}

	acct.CreditBalance.Sub(&acct.CreditBalance, amt)
	return true
}

func (acct *LiteTokenAccount) ParseTokenUrl() (*url.URL, error) {
	return url.Parse(acct.TokenUrl)
}
