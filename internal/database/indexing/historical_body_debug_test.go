// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build debug

package indexing_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// Under -tags debug the observer collapses an account's state components into
// one hash, so there is no path from a main state to its BPT entry: nothing is
// retained, and no historical answer may claim a main-state start or carry a
// body. It answers with the entry-rooted proof, which still validates.
func TestHistoricalBody_DebugBuildServesNoBody(t *testing.T) {
	sim, lite := changingLite(t, 10_000, 4)

	View(t, sim.DatabaseFor(lite), func(batch *database.Batch) {
		account := batch.Account(lite)
		blocks, err := account.RetainedStateReceiptBlocks().Get()
		require.NoError(t, err)
		require.Empty(t, blocks, "the debug observer has no main-state path, so nothing should be retained")

		retained, err := indexing.RetainedBlockRange(bvn0, batch)
		require.NoError(t, err)
		require.False(t, retained.IsEmpty(), "BPT history itself is retained under -tags debug")

		proof, err := indexing.HistoricalAccountStateProof(bvn0, batch, account, retained.Latest)
		require.NoError(t, err)
		require.False(t, proof.StartsAtMainState)
		require.Nil(t, proof.State)
		require.True(t, proof.Receipt.Validate(nil))
	})
}
