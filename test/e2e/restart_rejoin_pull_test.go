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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestRestartedNodeWithAPopulatedDatabaseResyncs is the RESTART case: a node
// that stops holding a real, peer-identical database at block R, while its
// peers run on to Q, and then rejoins through the production pull.
//
// TestJoinPullsFromPeersAndPromotes cannot catch what this catches, because it
// joins from emptyDb(): almost every account it needs is COLD, and a cold
// account is served with a receipt as of an old block the Directory has long
// since anchored, so it settles on the first try and carries the join. A
// restarted node already holds every cold account. It differs from its peers
// only in the accounts that change every block -- the partition's ledger,
// anchors and synthetic ledger -- and those are served as of the peer's
// CURRENT block, which the Directory has not anchored yet and which falls
// further behind every round. So the restart case has nothing that can settle,
// and the empty-database case never exercises that.
//
// This test asserts the one thing a restart must do: the node's local root
// becomes a root the Directory anchored, so it can hand off and execute again.
// It drives join.PulledState through join.QueryPeers -- the wiring
// cmd/accumulated/run/dagbft.go uses -- against the restarted node's own
// database, with that node's peer ID excluded exactly as production excludes
// it (#4303).
// # Why it is skipped, and what un-skips it (#4361)
//
// This is the gate for #4361, and #4361 alone does not open it. #4361 is the
// SERVING half: a peer can now be asked for an account or a BPT page as of a
// block the Directory anchored, and answers with a body and a receipt that
// terminate at that block's StateTreeAnchor. The PULL still asks for the
// peer's current block -- it sends no ForHeight -- so a restart still has
// nothing that can settle and this test still fails exactly as it did before.
// Run on this branch, 2026-09-19:
//
//	its local root is now 4b3dba55..., and its ledger record says block 89
//	the peers are at block 269 and the Directory has anchored BVN0 through 67
//
// #4301 — the spine validated by signature, so the join has one verified root
// per anchored block to ask AT — has since merged into the lead branch. What
// remains is #4362: the convergence loop, which asks for each account at that
// block and settles it on the round it was fetched, with the settle bound
// (maxSettleRounds, join/state.go) removed because there is nothing left to
// wait for.
//
// When they land, this test needs one more line than it has: the simulator
// must run with simulator.BPTHistoryDepth set, or the peers retain nothing and
// refuse every anchored-block request. A node's own default is 1024
// (cmd/accumulated/run/dagbft.go); the simulator's is zero, matching a node
// configured off.
func TestRestartedNodeWithAPopulatedDatabaseResyncs(t *testing.T) {
	// The one thing that flips this is the pull asking at an anchored block.
	// Named rather than described, so that whoever writes it can find this by
	// grep and so that a reader is not sent to a merged issue.
	t.Skip("#4362: the pull does not send ForHeight yet, so a restart still has nothing that can settle; " +
		"serving at an anchored block (#4361) is in, and this test also needs simulator.BPTHistoryDepth set when it is un-skipped")

	const joiner = 2 // the node that stops and comes back

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// A joining node executes nothing, so it reports no result for the blocks
	// it collects and the simulator's per-block comparison would call that a
	// consensus failure. It is not: the node is not executing on purpose.
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// Traffic, so the partition is at a non-trivial height with real state.
	for i := uint64(1); i <= 5; i++ {
		send(i)
	}
	sim.StepN(10)

	part := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	require.Equal(t, 3, p.NodeCount())

	// The node stops here. Its state at this moment is the network's state:
	// every node's root is the same, so what it holds is a block the Directory
	// anchors. A join must carry it FORWARD from here, never off this series.
	r := partitionBlock(t, p.NodeDatabase(joiner), part)
	rootAtStop := bptRoot(t, p.NodeDatabase(joiner))
	for i := 0; i < p.NodeCount(); i++ {
		require.Equal(t, r, partitionBlock(t, p.NodeDatabase(i), part),
			"precondition: every node is at the same block when one stops")
		require.Equal(t, rootAtStop, bptRoot(t, p.NodeDatabase(i)),
			"precondition: node %d holds the same state as the node that stops", i)
	}
	t.Logf("node %d stops at block R=%d with root %x", joiner, r, rootAtStop)

	// It restarts: staging is memory and a restart loses it, so from here the
	// node collects the blocks it is handed and executes none of them.
	p.RestartNode(joiner)
	require.True(t, p.Joining(joiner), "the restarted node should be collecting, not executing")

	// The network runs on without it.
	for i := uint64(6); i <= 15; i++ {
		send(i)
	}
	sim.StepN(30)

	q := partitionBlock(t, p.NodeDatabase(0), part)
	require.Equal(t, r, partitionBlock(t, p.NodeDatabase(joiner), part),
		"precondition: the restarted node executes nothing, so its database stays at R")
	require.Greater(t, q, r, "precondition: the peers moved on while it was away")
	t.Logf("the peers moved on to Q=%d while the restarted node stayed at R=%d", q, r)

	// The production wiring, with this node's own peer ID dropped from every
	// list: a joining node must never pull from itself (#4303).
	ctx := context.Background()
	sources := &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
		Self:    p.NodePeerID(joiner),
	}

	// The exclusion is real, not a peer ID that happens to match nothing: the
	// partition has three nodes and the pull must see the other two. Without
	// this the node could answer its own reads from the stale store it is
	// trying to replace, and the test would prove nothing (#4303).
	srcs, srcPart, err := sources.For(ctx, alice.JoinPath("tokens"))
	require.NoError(t, err)
	require.Equal(t, part, srcPart)
	require.Len(t, srcs, p.NodeCount()-1,
		"the joining node must be excluded from its own peer list, leaving the other two")

	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  p.NodeDatabase(joiner),
		Sources:   sources,
	})
	require.NoError(t, err)

	var matchedAt uint64
	var matched bool
	for round := 0; round < 60 && !matched; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		if round == 0 {
			t.Logf("after the first pull the local root is %x (changed by the spine pull: %v)",
				bptRoot(t, p.NodeDatabase(joiner)), bptRoot(t, p.NodeDatabase(joiner)) != rootAtStop)
		}

		// The network runs on while the node pulls, which is the case a
		// restart always meets: the peers are at a later block every time.
		sim.StepN(3)

		matchedAt, matched, err = state.Matched(ctx)
		require.NoError(t, err)
	}

	require.True(t, matched,
		"the restarted node never reached a root the Directory anchored, so it can never hand off and execute again.\n"+
			"  it stopped at block %d holding root %x, which every peer held too\n"+
			"  its local root is now %x, and its ledger record says block %d\n"+
			"  the peers are at block %d and the Directory has anchored BVN0 through block %d\n"+
			"  nothing it pulled ever settled: it needs only the accounts that change every block, and those are\n"+
			"  served as of the peer's current block, which is never anchored within the settle window",
		r, rootAtStop, bptRoot(t, p.NodeDatabase(joiner)), partitionBlock(t, p.NodeDatabase(joiner), part),
		partitionBlock(t, p.NodeDatabase(0), part), directoryAnchoredThrough(t, sim, "BVN0"))
	require.NotZero(t, matchedAt)
}

// partitionBlock is the block a node's state is: the record
// join.PulledState.localBlock reads.
func partitionBlock(t *testing.T, db *database.Database, part *url.URL) uint64 {
	t.Helper()
	batch := db.Begin(false)
	defer batch.Discard()
	var ledger *SystemLedger
	if err := batch.Account(part.JoinPath(Ledger)).Main().GetAs(&ledger); err != nil {
		return 0
	}
	return ledger.Index
}

// directoryAnchoredThrough is the highest block of a partition the Directory
// has anchored: the frontier a pulled account has to be settled against.
func directoryAnchoredThrough(t *testing.T, sim *Sim, partition string) uint64 {
	t.Helper()
	batch := sim.S.Partition("Directory").NodeDatabase(0).Begin(false)
	defer batch.Discard()
	var ledger *AnchorLedger
	if err := batch.Account(DnUrl().JoinPath(AnchorPool)).Main().GetAs(&ledger); err != nil {
		return 0
	}
	for _, seq := range ledger.Sequence {
		if seq.Url.Equal(PartitionUrl(partition)) {
			return seq.Delivered
		}
	}
	return 0
}
