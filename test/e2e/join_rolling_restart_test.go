// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
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

// TestARollingRestartOfTheDirectoryLeavesItsAnchorsServable — #4416. Every
// Directory validator restarts in turn, inside one 1024-entry window of
// dn.acme/anchors, and rejoins by the production pull while the network runs
// on under load. Each rejoins with an empty buffer, the shape a daemon
// reaches through a join buffer overrun (#4407) and, after #4405, through an
// outage longer than DAGGCDepth; see joinByPull. Afterwards every anchor on the pool is held by pull on at
// least one of them, and each node holds by pull the anchors appended while it
// was down.
//
// Before #4416 the pull brought no anchor's signature history, and since #4413
// a node refuses with NotReady a page of the pool holding an anchor it cannot
// serve signed. The pool here is one page, and every node holds some of it
// only by pull, so every node refused it: a fresh node could neither pull the
// Directory's spine (the pull asks for the pool's main chain expanded) nor
// read its roots from it (the anchor source reads the same page), and waited
// until the window moved on.
//
// Afterwards, through the production wiring and with nothing done here on
// their behalf, a fresh Directory node pulls the Directory's spine,
// dn.acme/anchors from its first entry, from the restarted validators and
// joins; and the anchor source a joining BVN0 node runs reads its roots from
// dn.acme/anchors through the Directory's peers. No node that executed every
// block is left to serve either. A page of the pool spans the ranges all
// three validators hold by pull, and a node refuses a whole page for one
// anchor it cannot serve signed (#4413), so before #4416 every validator
// refused it.
//
// A fresh BVN0 node is not joined here: on this tree it pulls, and then fails
// every handoff seeding its synthetic cache, because a message behind an
// entry of its anchor pool is not held -- with or without #4416 (reported on
// #4416).
func TestARollingRestartOfTheDirectoryLeavesItsAnchorsServable(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	net, _ := networkWithAFollower(t.Name(), 1, 3)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
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

	dnPart := sim.S.Partition(Directory)
	bvnPart := sim.S.Partition("BVN0")
	const validators = 3
	const fresh = 3 // networkWithAFollower appends it after the validators
	require.Equal(t, validators+1, dnPart.NodeCount(), "precondition: the follower has a Directory node")
	require.Equal(t, validators+1, bvnPart.NodeCount())

	// The fresh node: its Directory half starts its join before any block, so
	// it has executed nothing beyond genesis, and serves nothing (NotReady).
	dnPart.RestartNode(fresh)

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := 0; i < 5; i++ {
		send()
	}
	sim.StepN(10)

	// joinByPull starts the node collecting again with nothing in hand and
	// runs its join as the daemon runs it, with the network stepping under
	// load after every pull round. It holds its own state and nothing after
	// it, so it cannot execute from there and pulls.
	//
	// That is not what a daemon restart after a short outage looks like: the
	// daemon keeps its buffer, and certificate catch-up fills it with every
	// round after R within DAGGCDepth, so such a node meets its own state and
	// pulls nothing -- which is the simulator's single RestartNode. The shape
	// here, a buffer holding nothing between R and the live frontier that
	// then fills, is what a daemon reaches today only through a join buffer
	// overrun (#4407), and after #4405 through any outage longer than
	// DAGGCDepth (Node.Rejoin); beyond DAGGCDepth today the buffer never
	// fills (#4405). So this test holds the pull's capacity, through the
	// production wiring, on the overrun path.
	joinByPull := func(p *simulator.Partition, partition string, node int) {
		t.Helper()
		p.RestartNode(node)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		const maxRounds = 200
		stepping := &steppingState{cancel: cancel, State: p.NodeJoinState(node), step: func(round int) {
			if round%5 == 0 {
				send()
			}
			sim.StepN(3)
			if round >= maxRounds {
				cancel()
			}
		}}
		settler, ok := p.NodeExecutor(node).(join.Settler)
		require.True(t, ok)
		outcome, err := join.Run(ctx, join.Options{
			Partition: partition,
			Buffer:    p.NodeJoin(node),
			Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(node), Database: p.NodeDatabase(node)},
			State:     stepping,
			Peers:     &join.APIPeers{Partition: partition, Client: sim.S.Services(), Network: t.Name()},
			Retry:     time.Millisecond,
		})
		require.NoError(t, err, "%s node %d did not join within %d pull rounds", partition, node, maxRounds)
		require.Equal(t, join.Joined, outcome)
		require.False(t, p.Joining(node))
		require.Equal(t, nodestate.StateActive, p.NodeJoinState(node).Machine().State())
	}

	// The rolling restart: each Directory validator in turn stops, the
	// network runs on without it, and it rejoins by pull.
	dn := DnUrl()
	pool := dn.JoinPath(AnchorPool)
	for node := 0; node < validators; node++ {
		r := partitionBlock(t, dnPart.NodeDatabase(node), dn)
		dnPart.RestartNode(node)
		require.True(t, dnPart.Joining(node))
		for i := 0; i < 4; i++ {
			send()
		}
		sim.StepN(5)
		down := partitionBlock(t, dnPart.NodeDatabase((node+1)%validators), dn)
		joinByPull(dnPart, Directory, node)
		t.Logf("Directory node %d stopped at %d, the network reached %d without it, and it joined at %d",
			node, r, down, partitionBlock(t, dnPart.NodeDatabase(node), dn))
	}

	// Premise: every validator holds anchors it did not execute -- an anchor
	// a node executed has a delivered status, and the pull brings none -- and
	// the pool is inside one window.
	var entries int64
	for node := 0; node < validators; node++ {
		var pulled int
		View(t, dnPart.NodeDatabase(node), func(batch *database.Batch) {
			head, err := batch.Account(pool).MainChain().Head().Get()
			require.NoError(t, err)
			entries = head.Count
			for i := int64(0); i < head.Count; i++ {
				h, err := batch.Account(pool).MainChain().Entry(i)
				require.NoError(t, err)
				st, err := batch.Transaction(h).Status().Get()
				require.NoError(t, err)
				if !st.Delivered() {
					pulled++
				}
			}
		})
		t.Logf("Directory node %d holds %d of %d pool entries only by pull", node, pulled, entries)
		require.NotZero(t, pulled, "premise: Directory node %d holds none of the pool by pull", node)
	}
	require.Less(t, entries, int64(1024), "premise: the rolling restart is inside one window")

	// The fresh node pulls the Directory's spine, dn.acme/anchors from its
	// first entry, from the restarted validators, and joins.
	joinByPull(dnPart, Directory, fresh)
	require.Equal(t, bptRoot(t, dnPart.NodeDatabase(0)), bptRoot(t, dnPart.NodeDatabase(fresh)),
		"the fresh Directory node does not stand at its peers' root")

	// A joining BVN0 node reads the roots it verifies its pulls against from
	// dn.acme/anchors through the Directory's peers (join.QueryPeers, as the
	// daemon wires its anchor source). Every one of them joined by pull.
	bvn := PartitionUrl("BVN0")
	authority, err := anchorsrc.FromStore(bvnPart.NodeDatabase(0), bvn)
	require.NoError(t, err)
	bvnPool, err := anchorsrc.PoolFor(bvn, authority.BvnNames())
	require.NoError(t, err)
	require.True(t, bvnPool.Equal(pool))
	peers := &join.QueryPeers{Client: sim.S.Services(), Network: t.Name()}
	readThrough := func(name string, querier api.Querier) {
		t.Helper()
		src, err := anchorsrc.New(querier, bvnPool, bvn, authority)
		require.NoError(t, err)
		refused := map[uint64]string{}
		verified := 0
		src.OnRefused = func(block uint64, err error) { refused[block] = err.Error() }
		src.OnAnchor = func(*url.URL, uint64, [32]byte) { verified++ }
		require.NoError(t, src.Read(context.Background()), "%s cannot serve dn.acme/anchors", name)
		require.Empty(t, refused, "%s serves anchors that do not verify", name)
		require.NotZero(t, verified)
		t.Logf("%s: %d BVN0 anchors, all verified", name, verified)
	}
	readThrough("the Directory's peers", peers.Querier(dn))

	// And each of them serves every BVN0 anchor on its pool, signed, through
	// its own querier behind its join's gate.
	for node := 0; node <= fresh; node++ {
		readThrough(fmt.Sprintf("Directory node %d", node), apiimpl.NewQuerier(apiimpl.QuerierParams{
			Partition: Directory,
			Database:  dnPart.NodeDatabase(node),
			NodeState: dnPart.NodeJoinState(node).Machine(),
		}))
	}
}
