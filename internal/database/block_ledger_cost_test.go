// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// meteredStore is a memory store that counts the bytes each commit writes.
type meteredStore struct {
	*memory.Database
	written []int // bytes written by each root commit, in order
}

func (m *meteredStore) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	inner := m.Database.Begin(prefix, writable)
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get:    inner.Get,
		Commit: func(entries map[[32]byte]memory.Entry) error {
			n := 0
			for _, e := range entries {
				if err := inner.Put(e.Key, e.Value); err != nil {
					return err
				}
				n += len(e.Value)
			}
			m.written = append(m.written, n)
			return inner.Commit()
		},
		ForEach: func(fn func(*record.Key, []byte) error) error { return inner.ForEach(fn) },
		Discard: inner.Discard,
	})
}

// The work of recording a block's ledger is bounded by the block's contents
// (executor spec, invariant 9): block 400 with a thousand entries must cost
// what block 1 with a thousand entries cost, not four hundred times it.
func TestBlockLedgerCostIsIndependentOfHeight(t *testing.T) {
	const blocks, perBlock = 400, 1000
	store := &meteredStore{Database: memory.New(nil)}
	db := New(store, nil)
	ledger := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)

	for i := uint64(1); i <= blocks; i++ {
		batch := db.Begin(true)
		bl := new(BlockLedger)
		bl.Index = i
		bl.Entries = make([]*protocol.BlockEntry, perBlock)
		for j := range bl.Entries {
			bl.Entries[j] = &protocol.BlockEntry{Account: url.MustParse(fmt.Sprintf("acc://acct-%d.acme/tokens", j)), Chain: "main", Index: i}
		}
		require.NoError(t, recordBlockLedger(batch.Account(ledger), bl))
		require.NoError(t, batch.Commit())
	}

	first := average(store.written[:10])
	last := average(store.written[blocks-10:])
	require.Lessf(t, float64(last), 1.5*float64(first),
		"bytes written per block grew with height: blocks 1-10 averaged %d bytes, blocks %d-%d averaged %d", first, blocks-9, blocks, last)
}

func average(v []int) int {
	n := 0
	for _, x := range v {
		n += x
	}
	return n / len(v)
}

// recordBlockLedger is what block end does to record a block's ledger: the
// record, keyed by block index, and its hash on the block-ledger chain.
func recordBlockLedger(ledger *Account, bl *BlockLedger) error {
	if err := ledger.BlockLedger(bl.Index).Put(bl); err != nil {
		return err
	}
	data, err := bl.MarshalBinary()
	if err != nil {
		return err
	}
	h := sha256.Sum256(data)
	return ledger.BlockLedgerChain().Inner().AddEntry(h[:], false)
}
