// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"strings"
)

func (e *TransactionChainEntry) Compare(f *TransactionChainEntry) int {
	v := e.Account.Compare(f.Account)
	if v != 0 {
		return v
	}
	return strings.Compare(e.Chain, f.Chain)
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
