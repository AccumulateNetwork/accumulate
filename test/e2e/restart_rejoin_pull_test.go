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
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
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
//
// # How it settles (#4362)
//
// The pull asks each peer for CURRENT state and sends no ForHeight. #4301,
// the spine validated by signature, gives the join verified signed anchors:
// each carries the StateTreeAnchor of the block that sent it and the root
// chain's anchor and height as of that block. A pass is written once the
// root its receipts end at is proven: it equals a verified anchor's
// StateTreeAnchor, or the bpt chain's history from one such root to it,
// read from the peers, hashes into the root chain anchor a later verified
// anchor signs (anchorsrc.ProveRoot). So a pass served at block N is proven
// once any anchor after N is verified, whether or not block N sent one, and
// there is no settle bound: a pass is held only until the next verified
// anchor, or dropped and fetched again when the history has passed it.
//
// #4361, a peer serving an account or a BPT page as of an anchored block, is
// a reader's capability the pull does not use. The test still runs the
// simulator with simulator.BPTHistoryDepth at a node's own default of 1024
// (cmd/accumulated/run/dagbft.go), which is what a node runs with; that
// option gates the per-block BPT retention #4361 serves from (block_end.go),
// not the bpt chain entries and receipts the history proof reads.
func TestRestartedNodeWithAPopulatedDatabaseResyncs(t *testing.T) {
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
		// A node's own default (cmd/accumulated/run/dagbft.go): peers retain
		// enough history to serve an account as of an anchored block.
		simulator.BPTHistoryDepth(1024),
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

	// The join's state RestartNode built, as the daemon builds it: its
	// sources are a QueryPeers like the one above, and its machine is what
	// the node's querier refuses by.
	state := p.NodeJoinState(joiner)
	require.NotNil(t, state)

	// The join, as the daemon runs it, on a partition that has gone IDLE: no
	// transaction is submitted from here, so the partition anchors only on
	// its heartbeat, and its head stands most of the time at a block that
	// sent no anchor. The pulled state cannot be matched by equality there.
	// The node hands off from the pulled state unproven, executes the quiet
	// blocks like any node, and is proven at the next heartbeat anchor
	// (executor spec, "Sync", "Execute, and repair on a mismatch"). Before
	// it, a join that only pulled records waited at the peer's head for an
	// anchor of that block that never came: 29 releases, every one at a block
	// that sent no anchor (#4438).
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	const maxRounds = 120
	stepping := &steppingState{cancel: cancel, State: state, step: func(round int) {
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err = join.Run(runCtx, join.Options{
		Partition: "BVN0",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)

	ad := state.Machine().Get()
	require.Equal(t, nodestate.StateActive, ad.State,
		"the restarted node never matched a root its partition signed on an idle partition, so it can never serve again.\n"+
			"  it stopped at block %d holding root %x; the peers are at block %d",
		r, rootAtStop, partitionBlock(t, p.NodeDatabase(0), part))
	require.NotZero(t, ad.VerifiedAnchor)
	require.False(t, p.Joining(joiner), "the joined node executes")
	t.Logf("the node was proven at block %d, executing on an idle partition", ad.SinceBlock)

	// Executing from there, it stays on its peers' root chain.
	sim.StepN(20)
	var anchors [][]byte
	for i := 0; i < p.NodeCount(); i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "node %d's root chain differs from node 0's after the join", i)
	}
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
