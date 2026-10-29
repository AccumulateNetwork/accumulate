// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAnAccountInASectionNotLoadedIsNotServedAsNoLeaf — #4496, executor spec,
// "Sync", §3a: "an account absent from a section not loaded is not yet known"
// and "an AccountStates answer serves it with no body, so the asker asks the
// next peer", naming the peers to ask. A store holding alice's section by its
// hash alone answers alice as not loaded -- the section and the peers to ask
// -- by AccountStates, never NoLeaf; an account that does not exist, in a
// section the store holds whole, is still NoLeaf.
func TestAnAccountInASectionNotLoadedIsNotServedAsNoLeaf(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	bob := protocol.AccountUrl("bob", "tokens")
	var nobody *url.URL
	for i := 0; ; i++ {
		nobody = protocol.AccountUrl("nobody", string(rune('a'+i)))
		if database.SectionOfAccount(nobody) != database.SectionOfAccount(alice) {
			break
		}
	}

	db := database.OpenInMemory(nil)
	b := db.Begin(true)
	require.NoError(t, b.Account(here.JoinPath(protocol.Ledger)).Main().Put(&protocol.SystemLedger{Url: here.JoinPath(protocol.Ledger), Index: 10}))
	for _, u := range []*url.URL{alice, bob} {
		require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(5)}))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	// alice's section by its hash alone, as a node still filling holds it.
	sec := database.SectionOfAccount(alice)
	require.NotEqual(t, sec, database.SectionOfAccount(bob), "precondition: alice and bob share a section")
	b = db.Begin(true)
	h, err := b.Account(alice).Hash()
	require.NoError(t, err)
	require.NoError(t, b.ForgetAccount(alice))
	require.NoError(t, b.BPT().Insert(sec.MarkerKey(), h[:]))
	require.NoError(t, b.Commit())

	q := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{
		NodeState: nodestate.Always{}, // executed or archived database, not a joining node
		Database:  db,
		Partition: "BVN0",
		Redirect:  func(context.Context) []string { return []string{"holder"} },
	})}
	check := func(t *testing.T, states []*api.AccountState) {
		t.Helper()
		by := map[string]*api.AccountState{}
		for _, st := range states {
			by[st.Url.String()] = st
		}
		a := by[alice.String()]
		require.NotNil(t, a, "alice was not answered")
		require.False(t, a.NoLeaf, "alice, in a section this node has not loaded, was served as having no leaf")
		require.True(t, a.NotLoaded, "alice was not served as not loaded")
		require.Equal(t, uint64(sec), a.Section)
		require.Equal(t, []string{"holder"}, a.Redirect, "the answer names no peer to ask instead")
		require.Nil(t, a.Main)
		if st, ok := by[bob.String()]; ok {
			require.False(t, st.NotLoaded)
			require.NotNil(t, st.Main, "bob, loaded, was not served")
		}
		if st, ok := by[nobody.String()]; ok {
			require.True(t, st.NoLeaf, "an account no loaded section holds was not served as having no leaf")
			require.False(t, st.NotLoaded)
		}
	}

	t.Run("AccountStates", func(t *testing.T) {
		rec, err := q.QueryAccountStates(context.Background(), here, &api.AccountStatesQuery{Accounts: []*url.URL{alice, bob, nobody}})
		require.NoError(t, err)
		require.Len(t, rec.States, 3)
		check(t, rec.States)
	})

}
