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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestJoinPullsFromPeersAndPromotes drives the join's state half the way the
// daemon drives it, and is the test the E11 suite did not have.
//
// What the old tests did not exercise, and this one does:
//
//   - The pull is built on join.QueryPeers, which finds the partition's query
//     service through FindService and addresses ONE PEER by ID -- the
//     production path. TestPullReachesTheAnchoredRoot took a direct
//     api.Querier2{Querier: sim.S.Services()} handle, so there was no
//     discovery, no per-peer addressing and no node that could answer for
//     itself (#4303).
//   - Nothing here calls batch.UpdateBPT(). The old test called it by hand at
//     two places, which is exactly the step the production pull omitted, so it
//     converged where the daemon could not (#4305).
//   - The set of accounts is the pull's own: it reads the block ledger from a
//     peer and adds the partition's ledger and synthetic accounts, which no
//     block's envelopes name (#4306). Nothing is handed in.
//
// It asserts the two things a join must do and never did on a real network:
// accounts are pulled, and the tracker matches the block whose anchored root
// the local root equals — which does not promote the node until it hands off
// there (#4385).
func TestJoinPullsFromPeersAndPromotes(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
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

	// Let the synthetic deliveries drain and the anchors catch up, so the
	// partition is quiet when the join starts. A queue that is non-empty at the
	// block a peer serves cannot be verified at all (#4298), which is a
	// separate hole and not this test's subject.
	sim.StepN(20)

	ctx := context.Background()
	part := PartitionUrl("BVN0")

	// The production wiring: a client whose dialer resolves a peer's query
	// service, a router, and this node's own peer ID dropped from every list.
	// This node is joining, so it is not one of the simulator's peers; the
	// guard that its own ID would be dropped is TestSelectPeers_NeverThisNode.
	sources := &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
	}

	// Every peer of BVN0 is reachable by name, and there is more than one:
	// a pull that could only reach "whichever peer the dialer favours" would
	// have nothing to move on to when one refuses.
	srcs, srcPart, err := sources.For(ctx, alice.JoinPath("tokens"))
	require.NoError(t, err)
	require.Equal(t, part, srcPart, "the account routed to the wrong partition")
	require.Len(t, srcs, 3, "each of the partition's nodes is addressable on its own")

	// What a joining node really holds before it asks anybody anything: its
	// own genesis network accounts. They are the keys it verifies anchors
	// against, and they are the one thing it must not take from a peer
	// (#4301). An empty store is not a case: every node loads genesis before
	// it joins, and a node with no key to start from cannot verify a root.
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	var q uint64
	var ok bool
	for round := 0; round < 40 && !ok; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)

		// The network runs on while the node pulls, which is the case the join
		// exists for: the peers are at a later block every time it asks.
		sim.StepN(3)

		q, ok, err = state.Matched(ctx)
		require.NoError(t, err)
	}

	require.True(t, ok,
		"the join never reached a root the Directory anchored: the local root did not become a block's root")
	require.NotZero(t, q)

	// A match is not ACTIVE: the node has not handed off and executes
	// nothing (#4385). join.Run promotes when its handoff succeeds, with the
	// root this match found as the verified anchor.
	require.Equal(t, nodestate.StateBooting, state.Machine().State(),
		"a match alone promoted the node; it is ACTIVE only once it hands off")
	state.Promote(q)
	require.Equal(t, nodestate.StateActive, state.Machine().State())
	require.Equal(t, q, state.Machine().Get().SinceBlock)
	require.Equal(t, bptRoot(t, local), state.Machine().Get().VerifiedAnchor,
		"the verified anchor is the root the match found")

	// And the state it holds is the partition's, not a subset: the accounts a
	// block changed as a side effect and the system accounts no envelope names.
	View(t, local, func(batch *database.Batch) {
		for _, u := range []*url.URL{
			part.JoinPath(Ledger),
			part.JoinPath(Synthetic),
			part.JoinPath(AnchorPool),
			alice.JoinPath("tokens"),
			bob.JoinPath("tokens"),
		} {
			_, err := batch.Account(u).Main().Get()
			require.NoError(t, err, "the join did not pull %v", u)
		}
	})
}

// TestBlockLedgerNamesWhatABlockChanged — the surface the changed set is read
// through, against real records.
//
// BlockQuery with EntryRange.Expand false answers from the block ledger with
// the (account, chain, index) triples alone. What matters is that the set
// derived from them is the one the root commits to: it names the accounts a
// block wrote, not the accounts its envelopes mentioned.
func TestBlockLedgerNamesWhatABlockChanged(t *testing.T) {
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

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	sim.StepN(5)

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	q := api.Querier2{Querier: sim.S.Services()}

	// Walk what the partition has executed and collect the union, the way a
	// joining node does.
	end := sim.S.BlockIndex("BVN0")
	seen := map[string]bool{}
	names := 0
	for block := uint64(1); block <= end; block++ {
		count, expand := uint64(512), false
		rec, err := q.QueryMinorBlock(ctx, part, &api.BlockQuery{
			Minor:      &block,
			EntryRange: &api.RangeOptions{Count: &count, Expand: &expand},
		})
		if err != nil {
			continue // an empty block records nothing
		}
		for _, e := range rec.Entries.Records {
			require.NotNil(t, e.Account, "a block ledger entry named no account")
			// An entry may name no chain: the block changed the account's
			// state without appending to it (#4437). It must still name an
			// index of zero, and no value.
			if e.Name == "" {
				require.Zero(t, e.Index, "an account entry carried a chain index")
			}
			require.Nil(t, e.Value,
				"Expand false loaded the entry's value; the point of it is that it does not")
			seen[e.Account.String()] = true
			names++
		}
	}
	require.NotZero(t, names, "no block recorded anything")
	require.True(t, seen[alice.JoinPath("tokens").String()],
		"the block ledger does not name the account the transaction moved")

	// The two the record used to leave out, which the join then added by
	// hand (#4306): the system ledger, written by the record itself, and the
	// synthetic ledger, which moves without a chain append on the receiving
	// side. The record names both now (#4437), and the join reads only it.
	require.True(t, seen[part.JoinPath(Ledger).String()],
		"the block ledger does not name the system ledger")
	require.True(t, seen[part.JoinPath(Synthetic).String()],
		"the block ledger does not name the synthetic ledger")
	require.True(t, join.Routable(part.JoinPath(Ledger)))
}
