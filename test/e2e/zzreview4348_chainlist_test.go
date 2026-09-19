// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview4348_TheChainListTheApiServes — meeting.unserved marks the node
// "beyond" on every chain it holds that the peer did not serve. If the API
// serves a different set of chains than an EXECUTED store holds, a node that
// is simply behind looks mixed (beyond on one, behind on another) and the
// account is refused. Every "node" in the branch's tests is a pulled copy,
// whose chain index came from the peer's own list, so none of them can see it.
func TestReview4348_TheChainListTheApiServes(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	for i := 0; i < 3; i++ {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(3)

	part := PartitionUrl("BVN0")
	db := sim.DatabaseFor(alice)
	src := peerServingPart(db, "BVN0")

	for _, u := range []*url.URL{
		part.JoinPath(Ledger), part.JoinPath(AnchorPool), part.JoinPath(Synthetic),
		alice.JoinPath("tokens"), alice, alice.JoinPath("book"), alice.JoinPath("book", "1"),
	} {
		var local []string
		require.NoError(t, db.View(func(b *database.Batch) error {
			cs, err := b.Account(u).Chains().Get()
			if err != nil {
				return err
			}
			for _, cm := range cs {
				local = append(local, strings.ToLower(cm.Name))
			}
			return nil
		}))
		served, err := src.QueryAccountChains(context.Background(), u, &api.ChainQuery{})
		require.NoError(t, err)
		var api []string
		if served != nil {
			for _, c := range served.Records {
				if c != nil && c.Name != "" {
					api = append(api, strings.ToLower(c.Name))
				}
			}
		}
		sort.Strings(local)
		sort.Strings(api)
		t.Logf("%v\n   executed store holds: %v\n   the api serves:       %v", u, local, api)
		var missing []string
		for _, n := range local {
			found := false
			for _, m := range api {
				if m == n {
					found = true
				}
			}
			if !found {
				missing = append(missing, n)
			}
		}
		if len(missing) > 0 {
			t.Errorf("**** %v: the api does not serve %v, which the node holds: unserved() will call the node BEYOND on those", u, missing)
		}
	}
}

// heightsOf is chainHeights for any beginner.
func heightsOf(t *testing.T, db database.Beginner, accounts []*url.URL) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	b := db.Begin(false)
	defer b.Discard()
	for _, u := range accounts {
		chains, err := b.Account(u).Chains().Get()
		require.NoError(t, err)
		for _, cm := range chains {
			c, err := b.Account(u).ChainByName(cm.Name)
			require.NoError(t, err)
			head, err := c.Head().Get()
			require.NoError(t, err)
			out[u.String()+"#"+cm.Name] = head.Count
		}
	}
	return out
}

// TestReview4348_APullIntoAnExecutedStore drives the pull the way a restart
// does: the node's store is one an executor wrote, not a pulled copy.
func TestReview4348_APullIntoAnExecutedStore(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	part := PartitionUrl("BVN0")
	send := func(ts uint64) {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	send(1)
	sim.StepN(3)

	// The peer: a copy of the network as it stands now.
	db := sim.DatabaseFor(alice)
	accounts := []*url.URL{
		part.JoinPath(Ledger), part.JoinPath(AnchorPool), part.JoinPath(Synthetic),
		alice.JoinPath("tokens"), bob.JoinPath("tokens"), alice, alice.JoinPath("book"), alice.JoinPath("book", "1"),
	}
	behind := emptyDb()
	require.NoError(t, pullStateOnlyFrom(t, peerServingPart(db, "BVN0"), behind, part, accounts))

	// The executed node runs on.
	send(2)
	send(3)
	sim.StepN(3)

	// Now pull into THE EXECUTED STORE from the behind copy, as a restart does.
	real := sim.S.Database("BVN0")
	before := heightsOf(t, real, accounts)
	for _, u := range accounts {
		b := real.Begin(true)
		p, err := pull.Fetch(context.Background(), peerServingPart(behind, "BVN0"), b, u,
			pull.Options{Mode: pull.ModeStateOnly, Partition: part}, false)
		if err != nil {
			t.Errorf("**** %v was refused: %v", u, err)
			b.Discard()
			continue
		}
		t.Logf("%v: past=%v", u, p.Past())
		require.NoError(t, p.Keep())
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
	}
	after := heightsOf(t, real, accounts)
	for k, v := range before {
		if after[k] < v {
			t.Errorf("**** SHORTENED %s: %d -> %d", k, v, after[k])
		}
	}
}
