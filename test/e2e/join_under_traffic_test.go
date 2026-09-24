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

// TestARestartFarBehindJoinsAPartitionThatMovesEveryBlock is #4411's shape: a
// validator restarts more than fifty blocks behind a partition that changes
// state in every block -- a send within the partition and a send to another
// partition, every block, before the restart, while it is away, and on every
// round of its join -- and it must match a signed anchor's root and hand off.
//
// It is the production join end to end: join.Run with the simulator node's
// buffer, the executor's stage, the state RestartNode built (join.QueryPeers
// with this node's own peer ID dropped) and the validators found through the
// API. The network moves between every pull round and within each one, every
// few accounts the join asks for, as a real network does while a node pulls
// (Partition.SetPullHook).
//
// The per-pass join (pass, oneRoot, ProveRoot) never converged here: every
// pass kept only the accounts served at its majority root and fetched the rest
// again, so the local state was a mixture no block held (#4411; runs
// 20260924T074702Z, 093936Z, 111811Z). The algorithm pulls the whole tree and
// every block-ledger record from the start of the pull, in order, and the one
// proof is the match (executor spec, "Sync", "The algorithm", steps 1-3).
func TestARestartFarBehindJoinsAPartitionThatMovesEveryBlock(t *testing.T) {
	const joiner = 1
	const behind = 60

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	carol := url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(carol, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeIdentity(t, sim.DatabaseFor(carol), carol, acctesting.GenerateKey(carol)[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Every block carries a send within BVN0 and a send from BVN0 to BVN1:
	// alice's account, carol's, the partition's ledger and synthetic ledger,
	// and the anchor pool all change in every block.
	var ts uint64
	var sts []*TransactionStatus
	traffic := func() {
		for _, to := range []*url.URL{carol, bob} {
			ts++
			sts = append(sts, sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(to, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)))
		}
		sim.Step()
	}
	for i := 0; i < 10; i++ {
		traffic()
	}

	p := sim.S.Partition("BVN0")
	part := PartitionUrl("BVN0")
	r := partitionBlock(t, p.NodeDatabase(joiner), part)
	p.RestartNode(joiner)
	require.True(t, p.Joining(joiner))

	// The network runs on without it, moving every block.
	for i := 0; i < behind; i++ {
		traffic()
	}
	q := partitionBlock(t, p.NodeDatabase(0), part)
	require.Equal(t, r, partitionBlock(t, p.NodeDatabase(joiner), part),
		"precondition: the restarted node executes nothing, so its database stays where it stopped")
	require.GreaterOrEqual(t, q-r, uint64(50), "precondition: the restarted node is 50 or more blocks behind")
	t.Logf("node %d stopped at block %d; the peers are at %d", joiner, r, q)

	// It comes back now. A node that was down collected nothing while it was
	// away, so the blocks since it stopped are not in its buffer: it cannot
	// replay them and must pull (the first RestartNode only stopped it
	// executing; this one is the process starting, with an empty buffer).
	p.RestartNode(joiner)

	// The partition moves WHILE the node pulls, not only between rounds: a
	// block, with its traffic, every few accounts the join asks for. Without
	// this the simulator stands still for the whole of a pull round and every
	// account is served at one block, which no running network does (#4411).
	pulled := 0
	p.SetPullHook(joiner, func() {
		pulled++
		if pulled%10 == 0 {
			traffic()
		}
	})
	defer p.SetPullHook(joiner, nil)

	// The join, as the daemon runs it, with traffic in every block of every
	// round.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 150
	stepping := &steppingState{step: func(round int) {
		traffic()
		traffic()
		if round >= maxRounds {
			cancel()
		}
	}}
	stepping.State = p.NodeJoinState(joiner)
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the node did not match and hand off within %d pull rounds of a partition moving every block", maxRounds)
	require.False(t, p.Joining(joiner), "the joined node executes")
	t.Logf("joined after %d pull rounds and %d accounts asked for", stepping.round, pulled)
	p.SetPullHook(joiner, nil)

	// Executing from there, it stays on its peers' root chain while the
	// traffic goes on.
	for i := 0; i < 10; i++ {
		traffic()
	}
	for _, st := range sts {
		sim.StepUntil(Txn(st.TxID).Succeeds())
	}
	sim.StepN(10)
	var anchors [][]byte
	for i := 0; i < p.NodeCount(); i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "BVN0 node %d's root chain differs from node 0's after the join", i)
	}
}
