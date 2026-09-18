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
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestReview_PredicateKeepsOnlyMain: an account restored with only its main
// state - no Chains metadata, no chain records. The rebuild must not choke.
func TestReview_PredicateKeepsOnlyMain(t *testing.T) {
	built, u := buildAccountWithLongMainChain(t, 300)
	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	err := database.Restore(restored, ioutil.NewBuffer(snap), &database.RestoreOptions{
		SkipHashCheck: true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			if e.Key.Get(0) != "Account" || !u.Equal(e.Key.Get(1).(*url.URL)) {
				return false, nil
			}
			return e.Key.Get(2) == "Main" || e.Key.Get(2) == "Url", nil
		},
	})
	require.NoError(t, err, "an account with no chains at all must restore")
}

// TestReview_PredicateDropsHeadOnly: the chain's mark points survive but its
// head does not, so Chains names a chain whose head reads as empty.
func TestReview_PredicateDropsHeadOnly(t *testing.T) {
	built, u := buildAccountWithLongMainChain(t, 300)
	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	err := database.Restore(restored, ioutil.NewBuffer(snap), &database.RestoreOptions{
		SkipHashCheck: true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			if e.Key.Get(0) != "Account" || !u.Equal(e.Key.Get(1).(*url.URL)) {
				return false, nil
			}
			if e.Key.Get(2) == "MainChain" && e.Key.Len() > 3 && e.Key.Get(3) == "Head" {
				return false, nil
			}
			return true, nil
		},
	})
	require.NoError(t, err, "a chain with no head must restore")
}

// TestReview_RebuildIsIdempotent restores the same snapshot twice into the same
// database. The second pass must not move any index.
func TestReview_RebuildIsIdempotent(t *testing.T) {
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	const n, repeatAt = 400, 9

	built := database.OpenInMemory(nil)
	batch := built.Begin(true)
	pool := new(protocol.AnchorLedger)
	pool.Url = anchorPool
	require.NoError(t, batch.Account(anchorPool).Main().Put(pool))
	c, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("root %d", i)))
		hashes = append(hashes, h[:])
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, c.AddEntry(hashes[repeatAt], false))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch.Discard()
	_ = big.NewInt(0)

	snap := collectSnapshot(t, built)
	restored := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap),
		&database.RestoreOptions{BatchRecordLimit: 31}))
	require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap),
		&database.RestoreOptions{BatchRecordLimit: 17}))

	got, err := anchorRootIndexOf(t, restored, hashes[repeatAt])
	require.NoError(t, err)
	require.Equal(t, int64(repeatAt), got, "a second restore must not move the index")
	for i, h := range hashes {
		g, err := anchorRootIndexOf(t, restored, h)
		require.NoErrorf(t, err, "entry %d", i)
		require.Equalf(t, int64(i), g, "entry %d", i)
	}
}
