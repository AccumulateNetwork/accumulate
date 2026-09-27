// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// collectStates asks for every page of one ChangedAccountStates answer and
// reassembles the accounts answered in parts.
func collectStates(t *testing.T, q api.Querier2, partition *url.URL, after uint64) (map[string]*api.AccountState, *api.AccountStatesRecord, int, int) {
	t.Helper()
	ctx := context.Background()
	out := map[string]*api.AccountState{}
	var cursor []byte
	var first *api.AccountStatesRecord
	var part []byte
	pages, parts := 0, 0
	for {
		rec, err := q.QueryChangedAccountStates(ctx, partition, &api.ChangedAccountStatesQuery{After: after, Cursor: cursor})
		require.NoError(t, err)
		pages++
		if first == nil {
			first = rec
		}
		require.Equal(t, first.Through, rec.Through, "T moved between pages of one answer")
		for _, st := range rec.States {
			out[strings.ToLower(st.Url.String())] = st
		}
		if p := rec.Part; p != nil {
			parts++
			require.Equal(t, uint64(len(part)), p.Offset, "a part does not continue the one before it")
			part = append(part, p.Data...)
			if uint64(len(part)) == p.Total {
				st := new(api.AccountState)
				require.NoError(t, st.UnmarshalBinary(part))
				out[strings.ToLower(st.Url.String())] = st
				part = nil
			}
		}
		if len(rec.Cursor) == 0 {
			break
		}
		cursor = rec.Cursor
		require.Less(t, pages, 10000, "the answer did not end")
	}
	require.Empty(t, part, "an account answered in parts was left unfinished")
	return out, first, pages, parts
}

// TestChangedAccountStatesOverTheWire — #4453, executor spec, "Sync", §3.
// Over the v3 message transport, a partition answers every account its
// block-ledger records name, each once, each checking against its own leaf,
// stamped with the block of the view it was read from; paged by a byte
// budget, with an account larger than a page answered in parts that
// reassemble into the same state.
func TestChangedAccountStatesOverTheWire(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(10)

	q := api.Querier2{Querier: sim.S.Services()}
	for _, part := range []string{protocol.Directory, "BVN0"} {
		partition := protocol.PartitionUrl(part)
		db := sim.S.Database(part)

		// What the records name, and the block, read from the store.
		var block uint64
		named := map[string]bool{}
		leaves := map[string][32]byte{}
		View(t, db, func(batch *database.Batch) {
			var ledger *protocol.SystemLedger
			require.NoError(t, batch.Account(partition.JoinPath(protocol.Ledger)).Main().GetAs(&ledger))
			block = ledger.Index
			for n := uint64(1); n <= block; n++ {
				_, entries, err := indexing.LoadBlockLedger(batch.Account(partition.JoinPath(protocol.Ledger)), n)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if !e.Account.Equal(protocol.UnknownUrl()) {
						k := strings.ToLower(e.Account.String())
						named[k] = true
						v, err := batch.BPT().Get(batch.Account(e.Account).Key())
						if err == nil {
							leaves[k] = [32]byte(v)
						}
					}
				}
			}
		})
		require.NotEmpty(t, named, "precondition: the records name accounts")

		whole, rec, pages, _ := collectStates(t, q, partition, 0)
		require.Equal(t, block, rec.Block, "%s: the answer is not stamped with the block of its view", part)
		require.Equal(t, block, rec.Through, "%s: the records' end is not the peer's block", part)
		require.Len(t, whole, len(named), "%s: the answer does not name each account the records name, once", part)
		for k, st := range whole {
			require.True(t, named[k], "%s: %s was answered and no record names it", part, k)
			if st.NoLeaf {
				continue
			}
			require.NoError(t, pull.CheckState(st), "%s: %s does not hash to its own leaf", part, k)
			require.Equal(t, leaves[k], st.LeafHash, "%s: %s's leaf is not the store's", part, k)
		}
		t.Logf("%s: %d accounts in %d page(s) at block %d", part, len(whole), pages, block)

		// A small budget: pages, and accounts in parts, that come to the same.
		restore := apiimpl.SetAccountStatesBudget(512)
		small, _, smallPages, parts := collectStates(t, q, partition, 0)
		restore()
		require.NotZero(t, parts, "%s: no account was answered in parts", part)
		require.Greater(t, smallPages, pages, "%s: a small budget did not page the answer", part)
		require.Len(t, small, len(whole))
		for k, st := range whole {
			a, err := st.MarshalBinary()
			require.NoError(t, err)
			b, err := small[k].MarshalBinary()
			require.NoError(t, err)
			require.Equal(t, a, b, "%s: %s differs when paged", part, k)
		}

		// AccountStates: the same state for named accounts, and no leaf for
		// an account the partition does not hold.
		var some []*url.URL
		for _, st := range whole {
			some = append(some, st.Url)
			if len(some) == 3 {
				break
			}
		}
		ghost := protocol.AccountUrl("ghost", "tokens")
		rec2, err := q.QueryAccountStates(context.Background(), partition, &api.AccountStatesQuery{Accounts: append(some, ghost)})
		require.NoError(t, err)
		require.Equal(t, block, rec2.Block)
		require.Len(t, rec2.States, len(some)+1)
		for _, st := range rec2.States[:len(some)] {
			a, _ := st.MarshalBinary()
			b, _ := whole[strings.ToLower(st.Url.String())].MarshalBinary()
			require.Equal(t, b, a)
		}
		require.True(t, rec2.States[len(some)].NoLeaf, "%s: an account the partition does not hold was answered with a leaf", part)

		// A BPT page carries the block of its view.
		page, err := q.QueryBptPage(context.Background(), partition, &api.BptPageQuery{Count: 4})
		require.NoError(t, err)
		require.Equal(t, block, page.Block, "%s: a BPT page is not stamped with the block of its view", part)
	}
}
