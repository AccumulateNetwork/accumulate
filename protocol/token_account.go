package protocol

import (
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type AccountWithTokens interface {
	Account
	TokenBalance() *big.Int
	CreditTokens(amount *big.Int) bool
	CanDebitTokens(amount *big.Int) bool
	DebitTokens(amount *big.Int) bool
	GetTokenUrl() *url.URL
}

var _ AccountWithTokens = (*TokenAccount)(nil)
var _ AccountWithTokens = (*LiteTokenAccount)(nil)

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
	page.CreditBalance += amount
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

func (acct *LiteTokenAccount) GetTokenUrl() *url.URL {
	return acct.TokenUrl
}

func (i *TokenIssuer) Issue(amount *big.Int) bool {
	i.Issued.Add(&i.Issued, amount)
	return i.SupplyLimit == nil || i.Issued.Cmp(i.SupplyLimit) <= 0
}
