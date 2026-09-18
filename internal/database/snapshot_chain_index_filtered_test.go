// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

// A restore that filters records out may keep a chain's head and drop the
// records its entries live in. That is legitimate, and the index rebuild must
// not turn it into a failed restore - but with no filter in play a missing entry
// is corruption and must still be loud.
//
// The first two tests here are the reviewer's, taken from review-4328 at
// 0926b032a (internal/database/zz_review_predicate_test.go). They pass at
// f9635bf0b and failed against the first cut of the rebuild.

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	recorddb "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// buildAccountWithLongMainChain makes an ordinary token account whose main chain
// is longer than one mark point (markPower 8 -> markFreq 256), which is what
// mainnet accounts look like.
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

// TestRestore_PredicateDropsMarkPoints reproduces what
// internal/node/genesis/extract.go does on its first pass: it keeps an account's
// Main and its MainChain Head, and DROPS the chain's mark points (States) for
// anything that is not a data account. A snapshot carries no Element records
// either - Element is an index record - so Chain.Entry cannot reconstruct an
// element below the last mark point, and the restore must still succeed.
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

// TestGenesisExtract_LongMainChain runs the real genesis.Extract, the caller the
// filter above was copied from. It is reachable from the released binary:
// `accumulated init prepare-genesis` (cmd_init_network.go prepareGenesis), the
// command that ingests mainnet snapshots to produce a genesis snapshot.
func TestGenesisExtract_LongMainChain(t *testing.T) {
	built, _ := buildAccountWithLongMainChain(t, 300)
	snap := collectSnapshot(t, built)

	db := database.OpenInMemory(nil)
	accounts, err := genesis.Extract(db, ioutil.NewBuffer(snap), func(*url.URL) bool { return true })
	require.NoError(t, err)
	require.NotEmpty(t, accounts)
}

// TestGenesisExtract_ShortMainChainIsStillIndexed is the other half: a chain
// whose entries survive the filter must still be indexed. Without it, "skip the
// chain on NotFound" could degrade into "never index anything" and no test would
// notice.
func TestGenesisExtract_ShortMainChainIsStillIndexed(t *testing.T) {
	const n = 10 // < markFreq, so the entries live in the head's hash list
	built, u := buildAccountWithLongMainChain(t, n)
	snap := collectSnapshot(t, built)

	db := database.OpenInMemory(nil)
	_, err := genesis.Extract(db, ioutil.NewBuffer(snap), func(*url.URL) bool { return true })
	require.NoError(t, err)

	batch := db.Begin(false)
	defer batch.Discard()
	c, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)
	require.Equal(t, int64(n), c.Height())
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("entry %d", i)))
		got, err := c.HeightOf(h[:])
		require.NoErrorf(t, err, "entry %d must be indexed", i)
		require.Equal(t, int64(i), got)
	}
}

// TestRestore_MissingEntryWithoutPredicateIsLoud guards the other direction. The
// tolerance above is scoped to a filtered restore. An unfiltered restore whose
// entries cannot be read is a corrupt database, and must not be waved through -
// otherwise the fix for #4328 becomes exactly the silent no-op it was written to
// remove.
//
// The snapshot is collected through a Collect predicate that drops the mark
// points, then restored with no Restore predicate at all.
func TestRestore_MissingEntryWithoutPredicateIsLoud(t *testing.T) {
	built, _ := buildAccountWithLongMainChain(t, 300)

	buf := new(ioutil.Buffer)
	rb := built.Begin(false)
	_, err := rb.Collect(buf, nil, &database.CollectOptions{
		Predicate: func(r recorddb.Record) (bool, error) {
			k := r.Key()
			if k.Len() > 3 && k.Get(2) == "MainChain" && k.Get(3) == "States" {
				return false, nil
			}
			return true, nil
		},
	})
	require.NoError(t, err)
	rb.Discard()

	restored := database.OpenInMemory(nil)
	err = database.Restore(restored, ioutil.NewBuffer(buf.Bytes()),
		&database.RestoreOptions{SkipHashCheck: true})
	require.Error(t, err, "an unfiltered restore must not hide an unreadable entry")
	require.Contains(t, err.Error(), "rebuild chain indexes")
}

// TestRestore_FilteredChainIsNotPartiallyIndexed is the stop-vs-skip decision.
//
// Chain.Entry succeeds for indices at or above the last mark point - those live
// in the head's hash list - and fails below it once the mark points are gone. So
// a rebuild that skipped an unreadable entry and carried on would index the tail
// while never scanning the history, and a hash whose FIRST occurrence is in that
// unscanned history would be recorded at its LATER position. That is precisely
// the wrong value the first-occurrence rule exists to prevent, arriving by the
// back door.
//
// Here entry 290 repeats entry 5. 5 is below the last mark point and 290 is
// above it, so skipping would record 290. Nothing on the chain may be indexed.
func TestRestore_FilteredChainIsNotPartiallyIndexed(t *testing.T) {
	const n = 300
	const early, late = 5, 290 // late >= lastMark (256) > early

	u := protocol.AccountUrl("foo.acme", "tokens")
	built := database.OpenInMemory(nil)
	batch := built.Begin(true)
	acct := new(protocol.TokenAccount)
	acct.Url = u
	acct.TokenUrl = protocol.AcmeUrl()
	acct.Balance = *big.NewInt(1000)
	require.NoError(t, batch.Account(u).Main().Put(acct))
	c, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("entry %d", i)))
		hashes = append(hashes, h[:])
		if i == late {
			// Repeat the early entry instead of a fresh one
			require.NoError(t, c.AddEntry(hashes[early], false))
			hashes[i] = hashes[early]
			continue
		}
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch.Discard()

	// The node that built the chain names the first occurrence
	rb := built.Begin(false)
	bc, err := rb.Account(u).MainChain().Get()
	require.NoError(t, err)
	require.Equal(t, int64(n), bc.Height())
	got, err := bc.HeightOf(hashes[early])
	require.NoError(t, err)
	require.Equal(t, int64(early), got, "the builder names the first occurrence")
	rb.Discard()

	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap), &database.RestoreOptions{
		SkipHashCheck: true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			if e.Key.Get(0) != "Account" || !u.Equal(e.Key.Get(1).(*url.URL)) {
				return false, nil
			}
			if e.Key.Get(2) == "MainChain" && e.Key.Len() > 3 && e.Key.Get(3) == "States" {
				return false, nil
			}
			return true, nil
		},
	}))

	rc := restored.Begin(false)
	defer rc.Discard()
	c2, err := rc.Account(u).MainChain().Get()
	require.NoError(t, err)
	require.Equal(t, int64(n), c2.Height(), "the chain head survives")

	_, err = c2.HeightOf(hashes[early])
	require.Error(t, err, "a chain whose history was filtered out must not be partially indexed")
	require.True(t, errors.Is(err, errors.NotFound), "and the absence must be a clean NotFound: %v", err)
}
