// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// TestReview_RestoreIntoABatch: Restore takes a Beginner, and
// snapshot.FullRestore / internal/bsn hand it a *Batch rather than a
// *Database. The rebuild begins and commits nested batches; make sure that
// path works and that the index is visible through the parent.
func TestReview_RestoreIntoABatch(t *testing.T) {
	built, hashes := buildChainWithRepeat(t)
	snap := collectSnapshot(t, built)

	db := database.OpenInMemory(nil)
	parent := db.Begin(true)
	defer parent.Discard()

	require.NoError(t, database.Restore(parent, ioutil.NewBuffer(snap),
		&database.RestoreOptions{BatchRecordLimit: 1, SkipHashCheck: true}))
	require.NoError(t, parent.Commit())

	for i, h := range hashes {
		got, err := anchorRootIndexOf(t, db, h)
		require.NoErrorf(t, err, "entry %d must be indexed", i)
		if i == 3 {
			require.Equal(t, int64(2), got, "the repeat must name the first occurrence")
		} else {
			require.Equal(t, int64(i), got, "entry %d", i)
		}
	}
}
