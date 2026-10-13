// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	kvldb "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// writeDataAccount writes a data account with n writeData transactions on its
// main chain through the model. With index, the data-entry index the queries
// read is written too; without it the store is what the 13 July 2025 restore
// carried (#4268): the transactions and the chain, not the index.
func writeDataAccount(t *testing.T, path string, u *url.URL, n int, index bool) {
	kv, err := kvldb.Open(path)
	require.NoError(t, err)
	defer kv.Close()
	batch := coredb.New(kv, nil).Begin(true)
	defer batch.Discard()

	require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	chain, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		entry := &protocol.DoubleHashDataEntry{Data: [][]byte{[]byte(fmt.Sprint("entry ", i))}}
		txn := &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: u},
			Body:   &protocol.WriteData{Entry: entry},
		}
		h := txn.ID().Hash()
		require.NoError(t, batch.Message(h).Main().Put(&messaging.TransactionMessage{Transaction: txn}))
		require.NoError(t, batch.Transaction(h[:]).Status().Put(&protocol.TransactionStatus{Code: errors.Delivered}))
		require.NoError(t, chain.AddEntry(h[:], false))
		if index {
			require.NoError(t, indexing.Data(batch, u).Put(entry.Hash(), h[:]))
		}
	}
	require.NoError(t, batch.Commit())
}

func openRO(t *testing.T, path string) *leveldb.DB {
	db, err := leveldb.OpenFile(path, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// The bridge's data queries fail on the live store, which has the
// transactions but not the index, and succeed with the sidecar behind it.
func TestCheckDataIndex(t *testing.T) {
	dir := t.TempDir()
	u := url.MustParse("acc://bridge.acme/tokens")
	writeDataAccount(t, filepath.Join(dir, "live"), u, 3, false)
	writeDataAccount(t, filepath.Join(dir, "sidecar"), u, 3, true)

	results := runChecks([]*leveldb.DB{openRO(t, filepath.Join(dir, "live"))}, openRO(t, filepath.Join(dir, "sidecar")), "", []*url.URL{u})
	require.Len(t, results, 2)

	live, both := results[0], results[1]
	require.Equal(t, "live", live.Layer)
	require.True(t, live.IsData)
	require.EqualValues(t, 3, live.Total)
	require.EqualValues(t, 3, live.Read, "the transactions are there")
	require.EqualValues(t, 0, live.DataTotal)
	require.ErrorContains(t, live.LatestErr, "account has no data entries", "the error #4268 reports")

	require.Equal(t, "live+sidecar", both.Layer)
	require.EqualValues(t, 3, both.Read)
	require.Empty(t, both.Wrong)
	require.NoError(t, both.LatestErr)
	require.NoError(t, both.DataErr)
	require.EqualValues(t, 3, both.DataTotal)
	require.EqualValues(t, 3, both.DataRead)
	require.Empty(t, both.DataWrong)
}
