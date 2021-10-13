package protocol

import "math/big"

func (ms *MultiSigSpec) Credit(amount uint64) {
	amt := new(big.Int)
	amt.SetUint64(amount)
	ms.CreditBalance.Add(&ms.CreditBalance, amt)
}

func (ms *MultiSigSpec) Debit(amount uint64) bool {
	amt := new(big.Int)
	amt.SetUint64(amount)
	if amt.Cmp(&ms.CreditBalance) > 0 {
		return false
	}

	ms.CreditBalance.Sub(&ms.CreditBalance, amt)
	return true
}
