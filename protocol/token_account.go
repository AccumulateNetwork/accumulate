package protocol

import (
	"errors"
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
	CreditCredits(amount uint64)
	DebitCredits(amount uint64) bool
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

func (ms *KeyPage) CreditCredits(amount uint64) {
	ms.CreditBalance = amount
}

func (ms *KeyPage) DebitCredits(amount uint64) bool {
	if amount > ms.CreditBalance {
		return false
	}

	ms.CreditBalance -= amount
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

func (acct *LiteTokenAccount) DebitCredits(amount uint64) bool {
	if amount > acct.CreditBalance {
		return false
	}
	acct.CreditBalance -= amount
	return true
}

func (acct *LiteTokenAccount) GetTokenUrl() *url.URL {
	return acct.TokenUrl
}

func (acct *LiteTokenAccount) SetNonce(key []byte, nonce uint64) error {
	// TODO Check the key hash?
	acct.Nonce = nonce
	return nil
}

func (page *KeyPage) SetNonce(key []byte, nonce uint64) error {
	ks := page.FindKey(key)
	if ks == nil {
		// Should never happen
		return errors.New("failed to find key spec")
	}

	ks.Nonce = nonce
	return nil
}
