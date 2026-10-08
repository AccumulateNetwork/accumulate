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

// collectAccountStates asks for the accounts' states, every page of the
// answer, reassembling an account answered in parts, and returns them by
// name with the pages and parts it took.
func collectAccountStates(t *testing.T, q api.Querier2, partition *url.URL, accounts []*url.URL) (map[string]*api.AccountState, *api.AccountStatesRecord, int, int) {
	t.Helper()
	ctx := context.Background()
	out := map[string]*api.AccountState{}
	var cursor []byte
	var first *api.AccountStatesRecord
	var part []byte
	pages, parts := 0, 0
	for {
		rec, err := q.QueryAccountStates(ctx, partition, &api.AccountStatesQuery{Accounts: accounts, Cursor: cursor})
		require.NoError(t, err)
		pages++
		if first == nil {
			first = rec
		}
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

// TestChangedAccountStatesOverTheWire: a ChangedAccountStates answer is the
// block-ledger records after After through T, one answer, naming every
// account those blocks changed and carrying no states (#4552); AccountStates
// serves the named accounts' states from one view, paged by a byte budget,
// each hashing to its own leaf.
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

		// The records: one answer, no states, no pages.
		rec, err := q.QueryChangedAccountStates(context.Background(), partition, &api.ChangedAccountStatesQuery{After: 0})
		require.NoError(t, err)
		require.Equal(t, block, rec.Block, "%s: the answer is not stamped with the block of its view", part)
		require.Equal(t, block, rec.Through, "%s: the records' end is not the peer's block", part)
		require.Empty(t, rec.States, "%s: the records answer carries states", part)
		require.Nil(t, rec.Part, "%s: the records answer carries a part", part)
		require.Empty(t, rec.Cursor, "%s: the records answer is paged", part)
		answered := map[string]*url.URL{}
		for _, r := range rec.Records {
			require.True(t, r.Index > 0 && r.Index <= block, "%s: a record of block %d, outside 1 through %d", part, r.Index, block)
			for _, u := range api.RecordNames(r.Entries) {
				answered[strings.ToLower(u.String())] = u
			}
		}
		require.Len(t, answered, len(named), "%s: the records do not name each account the store's records name, once", part)
		for k := range answered {
			require.True(t, named[k], "%s: %s is named and no record of the store names it", part, k)
		}
		var accounts []*url.URL
		for _, u := range answered {
			accounts = append(accounts, u)
		}

		// The states the records lead to, from one view, each hashing to
		// its own leaf.
		whole, first, pages, _ := collectAccountStates(t, q, partition, accounts)
		require.Equal(t, block, first.Block, "%s: a states answer is not stamped with the block of its view", part)
		require.Len(t, whole, len(named), "%s: the states answer does not serve each account the records name, once", part)
		for k, st := range whole {
			require.True(t, named[k], "%s: %s was answered and no record names it", part, k)
			if st.NoLeaf {
				continue
			}
			require.NoError(t, pull.CheckState(st), "%s: %s does not hash to its own leaf", part, k)
			require.Equal(t, leaves[k], st.LeafHash, "%s: %s's leaf is not the store's", part, k)
		}
		t.Logf("%s: %d records naming %d accounts, served in %d page(s) at block %d", part, len(rec.Records), len(whole), pages, block)

		// A small budget: pages, and accounts in parts, that come to the same.
		restore := apiimpl.SetAccountStatesBudget(512)
		small, _, smallPages, parts := collectAccountStates(t, q, partition, accounts)
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

		// No leaf for an account the partition does not hold.
		ghost := protocol.AccountUrl("ghost", "tokens")
		rec2, err := q.QueryAccountStates(context.Background(), partition, &api.AccountStatesQuery{Accounts: append(accounts[:3:3], ghost)})
		require.NoError(t, err)
		require.Equal(t, block, rec2.Block)
		require.Len(t, rec2.States, 4)
		require.True(t, rec2.States[3].NoLeaf, "%s: an account the partition does not hold was answered with a leaf", part)

		// A BPT page carries the block of its view.
		page, err := q.QueryBptPage(context.Background(), partition, &api.BptPageQuery{Count: 4})
		require.NoError(t, err)
		require.Equal(t, block, page.Block, "%s: a BPT page is not stamped with the block of its view", part)
	}
}
