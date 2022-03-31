package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestBatchCommit(t *testing.T) {
	db := OpenInMemory(nil)
	ledgerUrl := protocol.DnUrl().JoinPath(protocol.Ledger)

	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.InternalLedger)
	ledger.Url = ledgerUrl
	ledger.KeyBook = ledgerUrl
	ledger.Index = 5
	require.NoError(t, batch.Account(ledgerUrl).PutState(ledger))
	require.NoError(t, batch.Commit())

	batch = db.Begin(true)
	defer batch.Discard()

	sub := batch.Begin(true)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).GetStateAs(&ledger))
	require.Equal(t, int64(5), ledger.Index)
	ledger.Index = 6
	require.NoError(t, sub.Account(ledgerUrl).PutState(ledger))
	require.NoError(t, sub.Commit())

	sub = batch.Begin(false)
	defer sub.Discard()
	ledger = nil
	require.NoError(t, sub.Account(ledgerUrl).GetStateAs(&ledger))
	require.Equal(t, int64(6), ledger.Index)
}
