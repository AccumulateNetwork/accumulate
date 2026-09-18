// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	recorddb "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// incompleteSnapshot is a snapshot whose chain mark points are missing - the
// shape of a snapshot produced by a broken collector, which is exactly the
// failure docs/operations/snapshot-restore-issues.md Issue 3 is still
// investigating. Restoring it must not silently produce an unindexed node.
func incompleteSnapshot(t *testing.T) []byte {
	t.Helper()
	built, _ := buildAccountWithLongMainChain(t, 300)
	buf := new(ioutil.Buffer)
	rb := built.Begin(false)
	defer rb.Discard()
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
	return buf.Bytes()
}

// TestR2_ProgressPredicateSilencesTheGuard is the hole the `filtered bool` flag
// leaves. It infers "records were deliberately left out" from "a Predicate was
// supplied", and tools/cmd/debug/snap_restore.go:68 supplies one that filters
// NOTHING - its sole purpose is to print progress, and it returns true for
// every record.
//
// So the same snapshot, restored with the same intent, gets two different
// safety policies depending only on whether the caller passed a progress
// callback. With nil options the restore fails loudly (the branch's own
// TestRestore_MissingEntryWithoutPredicateIsLoud). With a
// returns-true-for-everything Predicate it succeeds and leaves the chain
// unindexed behind one slog.Info line - which is the silent divergence #4328
// exists to remove, on the one operator-facing restore tool.
func TestR2_ProgressPredicateSilencesTheGuard(t *testing.T) {
	snap := incompleteSnapshot(t)

	// 1. No predicate: loud, as the branch intends.
	strict := database.OpenInMemory(nil)
	err := database.Restore(strict, ioutil.NewBuffer(snap),
		&database.RestoreOptions{SkipHashCheck: true})
	require.Error(t, err, "baseline: an unfiltered restore of this snapshot fails")
	require.Contains(t, err.Error(), "rebuild chain indexes")

	// 2. snap_restore.go's predicate verbatim in shape: restore everything,
	//    the callback exists only to print.
	var calls int
	lenient := database.OpenInMemory(nil)
	err = database.Restore(lenient, ioutil.NewBuffer(snap), &database.RestoreOptions{
		SkipHashCheck: true,
		Predicate: func(r *sv2.RecordEntry) (bool, error) {
			calls++
			// The sole purpose of this function is to print progress.
			return true, nil
		},
	})
	require.Greater(t, calls, 0, "the predicate must have been consulted")
	require.NoError(t, err,
		"SHIPPED BEHAVIOUR: the same snapshot restores clean because a progress callback was passed")

	// And the resulting database is the unindexed one #4328 is about.
	b := lenient.Begin(false)
	defer b.Discard()
	c, err := b.Account(protocol.AccountUrl("foo.acme", "tokens")).MainChain().Get()
	require.NoError(t, err)
	require.Equal(t, int64(300), c.Height(), "the chain is there")
	h := sha256.Sum256([]byte("entry 0"))
	_, err = c.HeightOf(h[:])
	require.Error(t, err, "and it has no element index - the #4328 condition, reached silently")
}
