// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// getBlockLedgerKeys enumerates the block ledger records of a system ledger
// account, from genesis to the ledger's height, so snapshots carry them. Only
// the system ledger holds them; any other account has none.
func (c *Account) getBlockLedgerKeys() ([]accountBlockLedgerKey, error) {
	main, err := c.Main().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return nil, nil
	case err != nil:
		return nil, errors.UnknownError.Wrap(err)
	}
	ledger, ok := main.(*protocol.SystemLedger)
	if !ok {
		return nil, nil
	}
	var keys []accountBlockLedgerKey
	for i := uint64(protocol.GenesisBlock); i <= ledger.Index; i++ {
		_, err := c.BlockLedger(i).Get()
		switch {
		case errors.Is(err, errors.NotFound):
			continue // an empty block has no ledger
		case err != nil:
			return nil, errors.UnknownError.Wrap(err)
		}
		keys = append(keys, accountBlockLedgerKey{Index: i})
	}
	return keys, nil
}
