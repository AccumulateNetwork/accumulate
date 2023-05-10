// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
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
