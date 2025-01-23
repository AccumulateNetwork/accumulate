// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

type AccountWithCredits interface {
	Account
	GetCreditBalance() uint64
	CreditCredits(amount uint64)
	DebitCredits(amount uint64) bool
	CanDebitCredits(amount uint64) bool
}

var _ AccountWithCredits = (*LiteIdentity)(nil)
var _ AccountWithCredits = (*KeyPage)(nil)

func (li *LiteIdentity) CreditCredits(amount uint64) {
	li.CreditBalance += amount
}

func (li *LiteIdentity) GetCreditBalance() uint64 {
	return li.CreditBalance
}

func (li *LiteIdentity) CanDebitCredits(amount uint64) bool {
	return amount <= li.CreditBalance
}

func (li *LiteIdentity) DebitCredits(amount uint64) bool {
	if !li.CanDebitCredits(amount) {
		return false
	}
	li.CreditBalance -= amount
	return true
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
