// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// oneSectionGate is a section gate holding one section not loaded, as a
// joined node's does while it fills. A block's load of it succeeds at once:
// the store holds the section, so execution is unchanged, and only the
// readers the gate refuses -- every batch that is not a block's -- see it.
type oneSectionGate struct {
	section bpt.Section
	on      atomic.Bool
}

func (g *oneSectionGate) AnyNotLoaded() bool                      { return g.on.Load() }
func (g *oneSectionGate) NotLoaded(s bpt.Section) bool            { return g.on.Load() && s == g.section }
func (g *oneSectionGate) Load(*database.Batch, bpt.Section) error { return nil }
func (g *oneSectionGate) Committed([]bpt.Section)                 {}
func (g *oneSectionGate) Discarded([]bpt.Section)                 {}
func (g *oneSectionGate) Opened(*database.Batch)                  {}

// TestTheDaemonsAPIIsRefusedASectionNotLoaded — #4497, executor spec, "Sync",
// §3a: "What is not loaded is not served; the node redirects". The join
// installs its section gate on the database the executor runs on
// (join.NewState, SetSectionGate). The daemon built its API's querier, and
// its consensus and sequencer services, each on a database.New of its own over
// the same store, whose gate was never set: blocks and validation were gated,
// the API was not, and it answered NotFound for an account it has not loaded
// -- the redirect (Querier.redirected) was unreachable.
//
// This runs the daemon's own start path (run.New on a netsim) and reads an
// account through its JSON-RPC API, as a client does. The gate is put on the
// database the executor runs on, where the join puts it.
func TestTheDaemonsAPIIsRefusedASectionNotLoaded(t *testing.T) {
	inst, api := startNetsimInstance(t, nil)
	bvn := protocol.PartitionUrl("BVN1")

	// The executor's database over BVN1's store: the one holding its ledger.
	var db *database.Database
	inst.databases.mu.Lock()
	for _, o := range inst.databases.dbs {
		b := o.db.Ungated().Begin(false)
		_, err := b.Account(bvn.JoinPath(protocol.Ledger)).Main().Get()
		b.Discard()
		if err == nil {
			db = o.db
		}
	}
	n := len(inst.databases.dbs)
	inst.databases.mu.Unlock()
	require.NotNil(t, db, "no database over BVN1's store")

	// One database per store: the Directory's and the BVN's (#4497). A
	// service that opened its own would be a third.
	require.Equal(t, 2, n, "the daemon opened more than one database over a store")

	// An account of BVN1 in a section none of the node's own system
	// accounts is in.
	system := map[bpt.Section]bool{}
	for _, u := range join.SystemAccounts(bvn) {
		system[database.SectionOfAccount(u)] = true
	}
	var account *url.URL
	b := db.Ungated().Begin(false)
	for s := bpt.Section(4095); account == nil; s-- {
		if !system[s] {
			pairs, _, err := b.BPT().GetRange(s.ScanStart(), 16)
			require.NoError(t, err)
			for _, kv := range pairs {
				u, ok := kv.Key.Get(1).(*url.URL)
				if ok && bpt.SectionOf(kv.Key.Hash()) == s {
					account = u
					break
				}
			}
		}
		if s == 0 {
			break
		}
	}
	b.Discard()
	require.NotNil(t, account, "BVN1 holds no account outside its system sections")

	client := jsonrpc.NewClient(api)
	ctx := context.Background()

	gate := &oneSectionGate{section: database.SectionOfAccount(account)}
	db.SetSectionGate(gate)
	t.Cleanup(func() { db.SetSectionGate(nil) })

	// Every section loaded: the account is answered.
	_, err := client.Query(ctx, account, &apiv3.DefaultQuery{})
	require.NoError(t, err, "%v was not answered with every section loaded", account)

	// Its section not loaded: refused, naming the section, never NotFound.
	gate.on.Store(true)
	_, err = client.Query(ctx, account, &apiv3.DefaultQuery{})
	require.Error(t, err, "the daemon's API answered %v in a section it has not loaded", account)
	require.False(t, errors.Is(err, errors.NotFound), "the daemon's API said %v does not exist: %v", account, err)
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	d, ok := database.NotLoadedOf(err)
	require.True(t, ok, "the refusal does not carry the redirect shape: %v", err)
	require.Equal(t, uint16(gate.section), *d.Section)

	// And loaded again: answered.
	gate.on.Store(false)
	_, err = client.Query(ctx, account, &apiv3.DefaultQuery{})
	require.NoError(t, err)
}
