// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// LoadBlockLedger reads the block ledger record written since the chain form
// was activated, and falls through to the per-block account written before it
// (executor spec, "The block ledger"). Nothing is migrated at activation, so
// the fall-through is what keeps every historical block query working.

func blockLedgerAccount(t *testing.T) (*database.Batch, *database.Account) {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	return batch, batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger))
}

func putAccountForm(t *testing.T, ledger *database.Account, index uint64, entries []*protocol.BlockEntry, at time.Time) {
	t.Helper()
	bl := new(protocol.BlockLedger)
	bl.Url = ledger.Url().JoinPath(strconv.FormatUint(index, 10))
	bl.Index = index
	bl.Time = at
	bl.Entries = entries
	require.NoError(t, ledger.Account(strconv.FormatUint(index, 10)).Main().Put(bl))
}

func TestLoadBlockLedger_PrefersTheRecordWhenPresent(t *testing.T) {
	_, ledger := blockLedgerAccount(t)
	alice := protocol.AccountUrl("alice")

	logTime := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	logEntries := []*protocol.BlockEntry{{Account: alice, Chain: "main", Index: 3}}
	require.NoError(t, ledger.BlockLedger(5).Put(
		&database.BlockLedger{Index: 5, Time: logTime, Entries: logEntries}))

	// A DIFFERENT answer in the account form, to prove which one was read.
	acctTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	putAccountForm(t, ledger, 5, []*protocol.BlockEntry{{Account: alice, Chain: "signature", Index: 9}}, acctTime)

	at, entries, err := LoadBlockLedger(ledger, 5)
	require.NoError(t, err)
	assert.True(t, at.Equal(logTime), "the record wins over the account form")
	require.Len(t, entries, 1)
	assert.Equal(t, "main", entries[0].Chain)
}

func TestLoadBlockLedger_FallsThroughToTheAccountWhenNoRecordExists(t *testing.T) {
	_, ledger := blockLedgerAccount(t)
	alice := protocol.AccountUrl("alice")

	// No record for block 5: it was written before activation, as an account.
	acctTime := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	acctEntries := []*protocol.BlockEntry{{Account: alice, Chain: "main", Index: 3}}
	putAccountForm(t, ledger, 5, acctEntries, acctTime)

	at, entries, err := LoadBlockLedger(ledger, 5)
	require.NoError(t, err)
	assert.True(t, at.Equal(acctTime),
		"a block with no record must fall through to the pre-activation account, or every historical block query breaks at activation")
	require.Len(t, entries, 1)
	assert.True(t, entries[0].Account.Equal(alice))
}

func TestLoadBlockLedger_ReturnsNotFoundWhenNeitherExists(t *testing.T) {
	_, ledger := blockLedgerAccount(t)

	_, _, err := LoadBlockLedger(ledger, 12345)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errors.NotFound),
		"a block that was never recorded in either form is NotFound, not a zero answer")
}

// Guard against the fixture accidentally testing the wrong URL shape: the
// account form lives at <partition>/ledger/<index>, which is what the
// pre-Jiuquan write path produced.
func TestLoadBlockLedger_ReadsTheAccountFormAtItsRealAddress(t *testing.T) {
	batch, ledger := blockLedgerAccount(t)
	alice := protocol.AccountUrl("alice")

	at := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	putAccountForm(t, ledger, 7, []*protocol.BlockEntry{{Account: alice, Chain: "main", Index: 1}}, at)

	// The same account is visible at its absolute URL.
	var bl *protocol.BlockLedger
	blUrl := protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger, "7")
	require.NoError(t, batch.Account(blUrl).Main().GetAs(&bl))
	require.Equal(t, uint64(7), bl.Index)

	_, entries, err := LoadBlockLedger(ledger, 7)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}
