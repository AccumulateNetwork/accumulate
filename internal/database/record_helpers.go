package database

import (
	"bytes"
	"strings"
)

func (e *SignatureEntry) Compare(f *SignatureEntry) int {
	// Only use the index for comparison so that only one entry per index is
	// allowed
	switch {
	case e.KeyEntryIndex < f.KeyEntryIndex:
		return -1
	case e.KeyEntryIndex > f.KeyEntryIndex:
		return +1
	default:
		return 0
	}
}

func (e *TransactionChainEntry) Compare(f *TransactionChainEntry) int {
	v := e.Account.Compare(f.Account)
	if v != 0 {
		return v
	}
	return strings.Compare(e.Chain, f.Chain)
}

func (u *ChainUpdate) Compare(v *ChainUpdate) int {
	c := u.Account.Compare(v.Account)
	if c != 0 {
		return c
	}
	return strings.Compare(u.Name, v.Name)
}

func (e *BlockStateSynthTxnEntry) Compare(f *BlockStateSynthTxnEntry) int {
	v := bytes.Compare(e.Transaction, f.Transaction)
	switch {
	case v != 0:
		return v
	case e.ChainEntry < f.ChainEntry:
		return -1
	case e.ChainEntry > f.ChainEntry:
		return +1
	default:
		return 0
	}
}
