package protocol

import (
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type TokenHolder interface {
	TokenBalance() *big.Int
	CreditTokens(amount *big.Int) bool
	CanDebitTokens(amount *big.Int) bool
	DebitTokens(amount *big.Int) bool
	GetTokenUrl() *url.URL
}

type CreditHolder interface {
	GetCreditBalance() uint64
	CreditCredits(amount uint64)
	DebitCredits(amount uint64) bool
	CanDebitCredits(amount uint64) bool
}

var _ TokenHolder = (*TokenAccount)(nil)
var _ TokenHolder = (*LiteTokenAccount)(nil)
var _ CreditHolder = (*KeyPage)(nil)
var _ CreditHolder = (*LiteTokenAccount)(nil)

func (acct *TokenAccount) TokenBalance() *big.Int {
	return &acct.Balance
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

func (acct *TokenAccount) GetTokenUrl() *url.URL {
	return acct.TokenUrl
}

func (page *KeyPage) CreditCredits(amount uint64) {
	page.CreditBalance = amount
}

func (page *KeyPage) GetCreditBalance() uint64 {
	return page.CreditBalance
}

func (page *KeyPage) CanDebitCredits(amount uint64) bool {
	return amount <= page.CreditBalance
}

func (page *KeyPage) DebitCredits(amount uint64) bool {
	if !page.CanDebitCredits(amount) {
		return false
	}

	page.CreditBalance -= amount
	return true
}

func (acct *LiteTokenAccount) TokenBalance() *big.Int {
	return &acct.Balance
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
	acct.CreditBalance += amount
}

func (acct *LiteTokenAccount) GetCreditBalance() uint64 {
	return acct.CreditBalance
}

func (acct *LiteTokenAccount) CanDebitCredits(amount uint64) bool {
	return amount <= acct.CreditBalance
}

func (acct *LiteTokenAccount) DebitCredits(amount uint64) bool {
	if !acct.CanDebitCredits(amount) {
		return false
	}
	acct.CreditBalance -= amount
	return true
}

func (acct *LiteTokenAccount) GetTokenUrl() *url.URL {
	return acct.TokenUrl
}
