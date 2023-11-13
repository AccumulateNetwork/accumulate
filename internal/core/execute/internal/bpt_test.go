// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccountState verifies that adding an entry to an ADI's directory listing changes the account's BPT entry.
func TestAccountState(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(databaseObserver{})
	batch := db.Begin(true)
	defer batch.Discard()
	adiurl := url.MustParse("acc://testadi1.acme")
	bookurl := url.MustParse("acc://testadi1.acme/testbook1")
	testurl := url.MustParse("acc://testurl")
	a := batch.Account(adiurl)
	acc := &protocol.ADI{Url: adiurl, AccountAuth: protocol.AccountAuth{Authorities: []protocol.AuthorityEntry{{Url: bookurl}}}}
	err := a.Main().Put(acc)
	require.NoError(t, err)
	h1, err := (&observedAccount{a, batch}).hashState()
	require.NoError(t, err)
	err = a.Directory().Add(testurl)
	require.NoError(t, err)
	h2, err := (&observedAccount{a, batch}).hashState()
	require.NoError(t, err)
	require.NotEqual(t, h1.Hash(), h2.Hash())
}

func TestEvents(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(databaseObserver{})
	ledgerUrl := protocol.DnUrl().JoinPath(protocol.Ledger)

	// Setup
	batch := db.Begin(true)
	defer batch.Discard()
	ledger := new(protocol.SystemLedger)
	ledger.Url = ledgerUrl
	ledger.Index = 5
	require.NoError(t, batch.Account(ledgerUrl).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	doesChangeBpt := func(msg string, fn func(events *database.AccountEvents)) {
		t.Helper()

		batch := db.Begin(true)
		defer batch.Discard()

		before, err := batch.BPT().GetRootHash()
		require.NoError(t, err)

		account := batch.Account(ledgerUrl)
		fn(account.Events())
		require.NoError(t, account.Commit())

		after, err := batch.BPT().GetRootHash()
		require.NoError(t, err)
		require.NotEqual(t, hex.EncodeToString(before[:]), hex.EncodeToString(after[:]), msg)

		require.NoError(t, batch.Commit())
	}

	doesChangeBpt("Expired backlog additions are tracked", func(events *database.AccountEvents) {
		err := events.Backlog().Expired().Add(url.MustParseTxID("acc://e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5@ACME"))
		require.NoError(t, err)
	})

	doesChangeBpt("Pending transaction additions are tracked", func(events *database.AccountEvents) {
		err := events.Major().Pending(1).Add(url.MustParseTxID("acc://e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5@ACME"))
		require.NoError(t, err)
	})

	doesChangeBpt("Held vote additions are tracked", func(events *database.AccountEvents) {
		vote := &protocol.AuthoritySignature{
			Authority: url.MustParse("foo"),
			TxID:      url.MustParseTxID("acc://e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5@ACME"),
		}
		err := events.Minor().Votes(1).Add(vote)
		require.NoError(t, err)
	})

	doesChangeBpt("Expired backlog removals are tracked", func(events *database.AccountEvents) {
		err := events.Backlog().Expired().Put(nil)
		require.NoError(t, err)
	})

	doesChangeBpt("Pending transaction removals are tracked", func(events *database.AccountEvents) {
		err := events.Major().Pending(1).Put(nil)
		require.NoError(t, err)
	})

	doesChangeBpt("Held vote removals are tracked", func(events *database.AccountEvents) {
		err := events.Minor().Votes(1).Put(nil)
		require.NoError(t, err)
	})
}
