// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// buildAccountWithLongMainChain makes an ordinary token account whose main
// chain is longer than one mark point (markPower 8 -> markFreq 256), which is
// what mainnet accounts look like.
func buildAccountWithLongMainChain(t *testing.T, n int) (*database.Database, *url.URL) {
	t.Helper()
	u := protocol.AccountUrl("foo.acme", "tokens")

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	acct := new(protocol.TokenAccount)
	acct.Url = u
	acct.TokenUrl = protocol.AcmeUrl()
	acct.Balance = *big.NewInt(1000)
	require.NoError(t, batch.Account(u).Main().Put(acct))

	c, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("entry %d", i)))
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db, u
}

// TestRestore_PredicateDropsMarkPoints reproduces exactly what
// internal/node/genesis/extract.go does on its first pass: it keeps an
// account's Main and its MainChain Head, and DROPS the chain's mark points
// (States) for anything that is not a data account. On main this restore
// succeeds. With the chain-index rebuild it is a hard error, because
// Chain.Entry cannot reconstruct an element below the last mark point without
// the mark point state.
func TestRestore_PredicateDropsMarkPoints(t *testing.T) {
	built, u := buildAccountWithLongMainChain(t, 300) // > markFreq (256)
	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	err := database.Restore(restored, ioutil.NewBuffer(snap), &database.RestoreOptions{
		SkipHashCheck: true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			if e.Key.Get(0) != "Account" {
				return false, nil
			}
			if !u.Equal(e.Key.Get(1).(*url.URL)) {
				return false, nil
			}
			// genesis/extract.go: drop mark points for non-data accounts
			if e.Key.Get(2) == "MainChain" && e.Key.Len() > 3 && e.Key.Get(3) == "States" {
				return false, nil
			}
			return true, nil
		},
	})
	require.NoError(t, err, "a restore that filtered out mark points must still succeed")
}

// TestGenesisExtract_LongMainChain runs the real genesis.Extract, the caller
// the filter above was copied from.
func TestGenesisExtract_LongMainChain(t *testing.T) {
	built, _ := buildAccountWithLongMainChain(t, 300)
	snap := collectSnapshot(t, built)

	db := database.OpenInMemory(nil)
	accounts, err := genesis.Extract(db, ioutil.NewBuffer(snap), func(*url.URL) bool { return true })
	require.NoError(t, err)
	require.NotEmpty(t, accounts)
}
