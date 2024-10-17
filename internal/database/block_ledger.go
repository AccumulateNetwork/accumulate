// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/indexing"
)

type BlockLedgerLog struct {
	*indexing.Log[*BlockLedger]
}

func (c *Account) newBlockLedger() *BlockLedgerLog {
	return &BlockLedgerLog{indexing.NewLog[*BlockLedger](c.logger.L, c.store, c.key.Append("BlockLedger"), 4<<10)}
}

func (b *BlockLedgerLog) Append(block uint64, ledger *BlockLedger) error {
	return b.Log.Append(record.NewKey(block), ledger)
}

func (b *BlockLedgerLog) Replace(block uint64, ledger *BlockLedger) error {
	return b.Log.Replace(record.NewKey(block), ledger)
}

func (b *BlockLedgerLog) Find(block uint64) indexing.Query[*BlockLedger] {
	return b.Log.Find(record.NewKey(block))
}
