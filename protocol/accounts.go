package protocol

import (
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
)

func (ms *SigSpec) CreditCredits(amount uint64) {
	amt := new(big.Int)
	amt.SetUint64(amount)
	ms.CreditBalance.Add(&ms.CreditBalance, amt)
}

func (ms *SigSpec) DebitCredits(amount uint64) bool {
	amt := new(big.Int)
	amt.SetUint64(amount)
	if amt.Cmp(&ms.CreditBalance) > 0 {
		return false
	}

	ms.CreditBalance.Sub(&ms.CreditBalance, amt)
	return true
}

func (acct *AnonTokenAccount) CreditTokens(amount *big.Int) bool {
	if amount == nil || amount.Sign() < 0 {
		return false
	}

	acct.Balance.Add(&acct.Balance, amount)
	return true
}

func (acct *AnonTokenAccount) CanDebitTokens(amount *big.Int) bool {
	return amount != nil && acct.Balance.Cmp(amount) >= 0
}

func (acct *AnonTokenAccount) DebitTokens(amount *big.Int) bool {
	if !acct.CanDebitTokens(amount) {
		return false
	}

	acct.Balance.Sub(&acct.Balance, amount)
	return true
}

func (acct *AnonTokenAccount) CreditCredits(amount uint64) {
	amt := new(big.Int)
	amt.SetUint64(amount)
	acct.CreditBalance.Add(&acct.CreditBalance, amt)
}

func (acct *AnonTokenAccount) DebitCredits(amount uint64) bool {
	amt := new(big.Int)
	amt.SetUint64(amount)
	if amt.Cmp(&acct.CreditBalance) > 0 {
		return false
	}

	acct.CreditBalance.Sub(&acct.CreditBalance, amt)
	return true
}

func (acct *AnonTokenAccount) NextTx() uint64 {
	c := acct.TxCount
	acct.TxCount++
	return c
}

func (acct *AnonTokenAccount) ParseTokenUrl() (*url.URL, error) {
	return url.Parse(acct.TokenUrl)
}
