// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestBptPageQueryOverTheWire walks the Directory's BPT through the v3 message
// transport — the query is marshalled, routed, dispatched and the record
// marshalled back — and checks the pages against the partition's database.
// This is the path a node pulling the state uses (executor.md, "Sync").
func TestBptPageQueryOverTheWire(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(5)

	ctx := context.Background()
	q := api.Querier2{Querier: sim.S.Services()}
	dn := protocol.DnUrl()

	// Page through the Directory in small pages, so the paging itself is
	// exercised rather than one page answering everything.
	got := map[[32]byte][32]byte{}
	accounts := map[[32]byte]string{}
	var lastRoot [32]byte
	var pages int
	start := bptproof.FullScanStart()
	for {
		page, err := q.QueryBptPage(ctx, dn, &api.BptPageQuery{StartHash: start, Count: 4})
		require.NoError(t, err)
		pages++
		for _, e := range page.Entries {
			require.NotContains(t, got, e.KeyHash, "the same leaf came back on two pages")
			got[e.KeyHash] = e.ValueHash
			if e.Account != nil {
				accounts[e.KeyHash] = e.Account.String()
			}
		}
		lastRoot = page.BptRoot
		if page.Done {
			break
		}
		start = page.NextStart
		require.Less(t, pages, 1000, "the scan did not terminate")
	}
	require.Greater(t, pages, 1, "the scan fit in one page; it does not exercise paging")

	// What came over the wire must be what the partition holds.
	want := map[[32]byte][32]byte{}
	var wantRoot [32]byte
	View(t, sim.S.Database(protocol.Directory), func(batch *database.Batch) {
		var err error
		wantRoot, err = batch.GetBptRootHash()
		require.NoError(t, err)

		it := batch.BPT().Iterate(256)
		for it.Next() {
			for _, v := range it.Value() {
				want[v.Key.Hash()] = [32]byte(v.Value)
			}
		}
		require.NoError(t, it.Err())
	})
	require.Equal(t, want, got, "the leaves served over the wire are not the leaves the partition holds")
	require.Equal(t, wantRoot, lastRoot, "the root served with the pages is not the partition's root")

	// The account URL rides with the leaf: without it a node pulling the
	// state would have to keep its own key-hash to URL map.
	named := make([]string, 0, len(accounts))
	for _, u := range accounts {
		named = append(named, u)
	}
	require.Contains(t, named, protocol.DnUrl().JoinPath(protocol.Ledger).String(),
		"the directory's ledger was not named by any page")
	require.Len(t, named, len(got), "a leaf came back without the account it belongs to")

	// The record survives the JSON form of the wire too.
	page, err := q.QueryBptPage(ctx, dn, &api.BptPageQuery{Count: 4})
	require.NoError(t, err)
	b, err := json.Marshal(page)
	require.NoError(t, err)
	var back *api.BptPageRecord
	require.NoError(t, json.Unmarshal(b, &back))
	require.True(t, page.Equal(back))
}

// TestBptPageQueryDefaultsAndCaps — an omitted start begins a fresh scan, and
// a count past the server's cap is answered with the server's default rather
// than letting a client ask for the whole tree in one call.
func TestBptPageQueryDefaultsAndCaps(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(5)

	ctx := context.Background()
	q := api.Querier2{Querier: sim.S.Services()}
	dn := protocol.DnUrl()

	fresh, err := q.QueryBptPage(ctx, dn, &api.BptPageQuery{Count: 4})
	require.NoError(t, err)
	explicit, err := q.QueryBptPage(ctx, dn, &api.BptPageQuery{StartHash: bptproof.FullScanStart(), Count: 4})
	require.NoError(t, err)
	require.True(t, fresh.Equal(explicit), "an omitted start hash did not begin a fresh scan")

	huge, err := q.QueryBptPage(ctx, dn, &api.BptPageQuery{Count: 1 << 20})
	require.NoError(t, err)
	require.LessOrEqual(t, len(huge.Entries), 4096)
}
