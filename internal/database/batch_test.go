// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestBatchCommit(t *testing.T) {
	db := OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})
	ledgerUrl := protocol.DnUrl().JoinPath(protocol.Ledger)

	// Setup
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = ledgerUrl
	ledger.Index = 5
	require.NoError(t, batch.Account(ledgerUrl).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	// Create a long-running batch
	batch = db.Begin(true)
	defer batch.Discard()

	// Load, update, and store the ledger, then commit
	sub := batch.Begin(true)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).Main().GetAs(&ledger))
	require.Equal(t, uint64(5), ledger.Index)
	ledger.Index = 6
	require.NoError(t, sub.Account(ledgerUrl).Main().Put(ledger))
	require.NoError(t, sub.Commit())

	// Reload the ledger in a new batch and verify that the update is seen
	sub = batch.Begin(false)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).Main().GetAs(&ledger))
	require.Equal(t, uint64(6), ledger.Index)
}

func TestGetBptRootHash(t *testing.T) {
	db := OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	ledgerUrl := protocol.DnUrl().JoinPath(protocol.Ledger)

	// Setup
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = ledgerUrl
	ledger.Index = 5
	require.NoError(t, batch.Account(ledgerUrl).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	// Get the BPT hash
	batch = db.Begin(false)
	defer batch.Discard()
	hashBefore, err := batch.GetBptRootHash()
	require.NoError(t, err)

	// Make a change (don't commit)
	batch = db.Begin(true)
	defer batch.Discard()
	ledger = new(protocol.SystemLedger)
	ledger.Url = ledgerUrl
	ledger.Index = 6
	require.NoError(t, batch.Account(ledgerUrl).Main().Put(ledger))

	// Pull the hash from the BPT. Because accounts haven't yet been committed,
	// this will be the same as the hash before.
	hashAfter, err := batch.BPT().GetRootHash()
	require.NoError(t, err)
	assert.Equal(t, hashBefore, hashAfter)

	// Pull through GetBptRootHash. Because this forces accounts to update their
	// BPT entry, this will be different.
	hashAfter, err = batch.GetBptRootHash()
	require.NoError(t, err)
	assert.NotEqual(t, hashBefore, hashAfter)

	// Commit
	require.NoError(t, batch.Commit())

	// Pull the hash both ways. Because the batch has been committed, both will
	// be different.
	hashAfter2, err := batch.BPT().GetRootHash()
	require.NoError(t, err)
	assert.Equal(t, hashAfter, hashAfter2)
	hashAfter2, err = batch.GetBptRootHash()
	require.NoError(t, err)
	assert.Equal(t, hashAfter, hashAfter2)
}

func TestResolveAccountKey(t *testing.T) {
	// Setup
	db := OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})
	account := protocol.AccountUrl("foo")
	require.NoError(t, db.Update(func(batch *Batch) error {
		return batch.Account(account).Main().Put(&protocol.UnknownAccount{
			Url: account,
		})
	}))

	noChange := func(key *record.Key, message string, args ...any) {
		batch := db.Begin(false)
		defer batch.Discard()
		resolved := batch.ResolveAccountKey(key)
		require.Equalf(t, key.String(), resolved.String(), message, args...)
	}

	didChange := func(key, expected *record.Key, message string, args ...any) {
		batch := db.Begin(false)
		defer batch.Discard()
		resolved := batch.ResolveAccountKey(key)
		require.Equalf(t, expected.String(), resolved.String(), message, args...)
	}

	kh := record.NewKey("Account", account).Hash()

	noChange(record.NewKey("Transaction", [32]byte{1}), "transaction key does not change")
	noChange(record.NewKey("Account", account, "Main"), "resolved account key does not change")
	didChange(record.NewKey(kh), record.NewKey("Account", account), "base account key is resolved")
	didChange(record.NewKey(kh, "Main"), record.NewKey("Account", account, "Main"), "main state account key is resolved")
}
