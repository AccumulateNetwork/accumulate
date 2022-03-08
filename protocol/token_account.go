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

	// Deprecated: use TokenUrl field
	ParseTokenUrl() (*url.URL, error)
}

type CreditHolder interface {
	CreditCredits(amount big.Int)
	DebitCredits(amount big.Int) bool
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

// Deprecated: use TokenUrl field
func (acct *TokenAccount) ParseTokenUrl() (*url.URL, error) {
	return acct.TokenUrl, nil
}

func (ms *KeyPage) CreditCredits(amount big.Int) {
	amt := amount
	ms.CreditBalance.Add(&ms.CreditBalance, &amt)
}

func (ms *KeyPage) DebitCredits(amount big.Int) bool {
	amt := amount
	if amt.Cmp(&ms.CreditBalance) > 0 {
		return false
	}

	ms.CreditBalance.Sub(&ms.CreditBalance, &amt)
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

func (acct *LiteTokenAccount) CreditCredits(amount big.Int) {
	amt := amount
	acct.CreditBalance.Add(&acct.CreditBalance, &amt)
}

func (acct *LiteTokenAccount) DebitCredits(amount big.Int) bool {
	amt := amount

	if amt.Cmp(&acct.CreditBalance) > 0 {
		return false
	}

	acct.CreditBalance.Sub(&acct.CreditBalance, &amt)
	return true
}

// Deprecated: use TokenUrl field
func (acct *LiteTokenAccount) ParseTokenUrl() (*url.URL, error) {
	return acct.TokenUrl, nil
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
