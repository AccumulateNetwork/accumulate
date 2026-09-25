// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// emptyDb is a node's data directory before it has any state.
func emptyDb() *database.Database {
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

func bptRoot(t *testing.T, db *database.Database) [32]byte {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	r, err := b.GetBptRootHash()
	require.NoError(t, err)
	return r
}

// TestPullSpineCarriesTheChains — the spine is pulled with its chain history,
// because that is what the node needs to verify the signatures on the anchors
// it verifies everything else against (executor.md, "Sync", step 3).
func TestPullSpineCarriesTheChains(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(10)

	ctx := context.Background()
	src := api.Querier2{Querier: sim.S.Services()}

	local := emptyDb()
	batch := local.Begin(true)
	spine := pull.SpineAccounts(DnUrl())
	require.Equal(t, pull.DnSpineAccounts(), spine)
	for _, u := range spine {
		require.NoError(t, pull.Account(ctx, src, batch, u, pull.Options{Mode: pull.ModeFullSpine}),
			"pull %v", u)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	// Every spine account's chains must match the Directory's, entry for
	// entry: a head alone would let the node hold an anchor pool it cannot
	// read back.
	for _, u := range spine {
		var want, got []*api.ChainRecord
		rec, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
		require.NoError(t, err, "chains of %v", u)
		want = rec.Records
		require.NotEmpty(t, want, "%v has no chains", u)

		View(t, local, func(b *database.Batch) {
			for _, w := range want {
				c, err := b.Account(u).ChainByName(w.Name)
				require.NoError(t, err)
				head, err := c.Head().Get()
				require.NoError(t, err)
				require.Equal(t, int64(w.Count), head.Count, "%v chain %s height", u, w.Name)
				got = append(got, w)
			}
		})
	}

	// And the pulled leaves are the Directory's leaves.
	View(t, sim.S.Database(Directory), func(peer *database.Batch) {
		View(t, local, func(mine *database.Batch) {
			for _, u := range spine {
				theirs, err := peer.Account(u).Hash()
				require.NoError(t, err)
				ours, err := mine.Account(u).Hash()
				require.NoError(t, err)
				require.Equal(t, theirs, ours, "leaf of %v", u)
			}
		})
	})
}
