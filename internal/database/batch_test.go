package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestBatchCommit(t *testing.T) {
	db := OpenInMemory(nil)
	ledgerUrl := protocol.DnUrl().JoinPath(protocol.Ledger)

	// Setup
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = ledgerUrl
	ledger.Index = 5
	require.NoError(t, batch.Account(ledgerUrl).PutState(ledger))
	require.NoError(t, batch.Commit())

	// Create a long-running batch
	batch = db.Begin(true)
	defer batch.Discard()

	// Load, update, and store the ledger, then commit
	sub := batch.Begin(true)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).GetStateAs(&ledger))
	require.Equal(t, uint64(5), ledger.Index)
	ledger.Index = 6
	require.NoError(t, sub.Account(ledgerUrl).PutState(ledger))
	require.NoError(t, sub.Commit())

	// Reload the ledger in a new batch and verify that the update is seen
	sub = batch.Begin(false)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).GetStateAs(&ledger))
	require.Equal(t, uint64(6), ledger.Index)
}

func TestBatchCommit2(t *testing.T) {
	db := OpenInMemory(nil)
	ledgerUrl := protocol.DnUrl().JoinPath(protocol.Ledger)

	// Setup
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = ledgerUrl
	ledger.Index = 5
	require.NoError(t, batch.Account(ledgerUrl).PutState(ledger))
	require.NoError(t, batch.Commit())

	// Create a long-running batch
	batch = db.Begin(true)
	defer batch.Discard()

	// Load, update, and store the ledger, then commit
	sub := batch.Begin(true)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).GetStateAs(&ledger))
	require.Equal(t, uint64(5), ledger.Index)
	ledger.Index = 6
	require.NoError(t, sub.Account(ledgerUrl).PutState(ledger))
	require.NoError(t, sub.Commit())

	// Reload the ledger in the original batch and verify that the update is seen
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).GetStateAs(&ledger))
	require.Equal(t, uint64(6), ledger.Index)
}
