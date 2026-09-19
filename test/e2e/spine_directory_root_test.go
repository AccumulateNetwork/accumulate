// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// simSources is the join's Sources over a simulator: every account is routed
// the way the node routes it, and every read is answered by the network
// rather than by the store the join is filling.
type simSources struct {
	sim *Sim
}

func (s simSources) For(_ context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	id, err := s.sim.S.Router().RouteAccount(account)
	if err != nil {
		return nil, nil, err
	}
	return []pull.Source{api.Querier2{Querier: s.sim.S.Services()}}, PartitionUrl(id), nil
}

func (s simSources) Querier(*url.URL) api.Querier { return s.sim.S.Services() }

// genesisOf copies the network accounts a node holds from its own genesis
// into an empty store. This is the node's OWN copy — what says who may sign
// an anchor — and it is the one thing a join must not take from a peer.
func genesisOf(t *testing.T, sim *Sim, partition string) *database.Database {
	t.Helper()
	part := PartitionUrl(partition)
	db := emptyDb()
	batch := db.Begin(true)
	defer batch.Discard()
	for _, name := range []string{Network, Globals} {
		u := part.JoinPath(name)
		var acct Account
		require.NoError(t, sim.S.Database(partition).View(func(b *database.Batch) error {
			return b.Account(u).Main().GetAs(&acct)
		}))
		require.NoError(t, batch.Account(u).Main().Put(acct))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db
}

// TestTheDirectorysOwnRootIsObtainedThroughTheJoin is (e) of #4301.
//
// **The Directory's own root lives in a BVN's anchor pool, never in its own.**
// A produced anchor is on the RECEIVING partition's pool, so a node that
// reads every partition's root from dn.acme/anchors — which is what
// pull.DirectoryAnchors was, constructed once at join/state.go:166 and asked
// for every partition at :444 — can never obtain the Directory's. The 03:52Z
// note on #4301 says that was inferred from the routing rule and never shown
// by a run; this is the run.
//
// Nothing about the source is built by hand here. join.NewState chooses the
// pool, reads the validator sets out of the node's own store, and verifies
// the signatures; the test supplies a network and a way to reach its peers,
// which is all the daemon supplies.
func TestTheDirectorysOwnRootIsObtainedThroughTheJoin(t *testing.T) {
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
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	send()
	send()

	ctx := context.Background()

	// A Directory node with nothing but its own genesis network accounts:
	// the keys it starts from, and no root of anybody's.
	local := genesisOf(t, sim, Directory)
	t.Cleanup(func() { _ = local.Close() })

	state, err := join.NewState(join.StateOptions{
		Partition: DnUrl(),
		Database:  local,
		Sources:   simSources{sim},
		// It has executed nothing of its own beyond genesis.
		ExecutedBlock: 1,
	})
	require.NoError(t, err)

	// Pull, stepping the network so the anchors this node's roots come from
	// are produced and executed at the BVN.
	var matched uint64
	for i := 0; i < 80 && matched == 0; i++ {
		require.NoError(t, state.Pull(ctx))
		block, ok, err := state.Matched(ctx)
		require.NoError(t, err)
		if ok {
			matched = block
			break
		}
		sim.Step()
	}

	require.NotZero(t, matched,
		"the Directory never matched a root it verified: its own anchors are in a BVN's pool, "+
			"and a join that cannot read them there can never obtain the Directory's root (#4301)")
	t.Logf("the Directory's state matched the root anchored for its block %d", matched)
}

// unsignedPool is a peer that serves a perfectly self-consistent anchor pool
// with the validator signatures taken off. Everything else about it is
// honest: the anchors are the real ones, the roots are the real roots, and
// the BPT pages, receipts and ledgers all agree with them. That is the point
// — a peer that is consistent with itself is indistinguishable from a
// correct one until somebody checks who signed.
type unsignedPool struct {
	inner    join.Sources
	pool     *url.URL
	stripped *int
}

func (u unsignedPool) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	return u.inner.For(ctx, account)
}

func (u unsignedPool) Querier(partition *url.URL) api.Querier {
	return stripSignatures{inner: u.inner.Querier(partition), pool: u.pool, stripped: u.stripped}
}

type stripSignatures struct {
	inner    api.Querier
	pool     *url.URL
	stripped *int
}

func (s stripSignatures) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	rec, err := s.inner.Query(ctx, scope, q)
	if err != nil || !scope.Equal(s.pool) {
		return rec, err
	}
	rr, ok := rec.(*api.RecordRange[api.Record])
	if !ok {
		return rec, nil
	}
	for _, r := range rr.Records {
		ce, ok := r.(*api.ChainEntryRecord[api.Record])
		if !ok {
			continue
		}
		mr, ok := ce.Value.(*api.MessageRecord[messaging.Message])
		if !ok || mr.Signatures == nil || len(mr.Signatures.Records) == 0 {
			continue
		}
		mr.Signatures = new(api.RecordRange[*api.SignatureSetRecord])
		*s.stripped++
	}
	return rr, nil
}

// TestAPeerConsistentWithItselfDoesNotPromoteTheNode is (b) of #4301.
//
// The peer serves the true anchors, the true roots and the true state, and
// only the signatures are missing. Before this issue the join recorded every
// StateTreeAnchor it could decode out of the pool and handed it to the
// tracker, so the root the whole pull was verified against was a number the
// peer sent — and the scheme proved only that the peer agreed with itself.
// The node must not promote.
func TestAPeerConsistentWithItselfDoesNotPromoteTheNode(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	for i := 0; i < 10; i++ {
		sim.Step()
	}

	ctx := context.Background()
	local := genesisOf(t, sim, Directory)
	t.Cleanup(func() { _ = local.Close() })

	stripped := 0
	state, err := join.NewState(join.StateOptions{
		Partition: DnUrl(),
		Database:  local,
		Sources: unsignedPool{
			inner:    simSources{sim},
			pool:     PartitionUrl("BVN0").JoinPath(AnchorPool),
			stripped: &stripped,
		},
		ExecutedBlock: 1,
	})
	require.NoError(t, err)

	for i := 0; i < 80; i++ {
		require.NoError(t, state.Pull(ctx))
		_, ok, err := state.Matched(ctx)
		require.NoError(t, err)
		require.False(t, ok,
			"a node promoted on a root nobody signed: the chain of trust terminates in the peer (#4301)")
		sim.Step()
	}

	require.NotZero(t, stripped,
		"no anchor was served to the node at all, so this test proves nothing")
	t.Logf("%d anchors were served with their signatures removed, and none of them promoted the node", stripped)
}
