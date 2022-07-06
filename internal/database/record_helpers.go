package database

import (
	"strings"
)

func (e *TransactionChainEntry) Compare(f *TransactionChainEntry) int {
	v := e.Account.Compare(f.Account)
	if v != 0 {
		return v
	}
	return strings.Compare(e.Chain, f.Chain)
}
