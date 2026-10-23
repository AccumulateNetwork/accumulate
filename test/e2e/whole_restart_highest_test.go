// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAWholeRestartResumesFromTheHighestState — #4501 (executor spec, "A
// partition that restarts as a whole resumes from its highest signed,
// validated state"). One BVN0 validator stops executing and falls behind
// while the others go on; then every validator restarts at once. The node
// that is behind must not resume on its own, lower block: it takes the
// state of the peers that hold the highest and ends at their block, root for
// root. Each of the others resumes from its own block or, when enough of its
// peers already serve to sign an anchor, joins from them: the exit is a race
// (executor spec, "A partition that restarts as a whole", 1; #4466), and run
// 1 of 30 locally had a Directory node find two of its three peers resumed and
// join from them at block 51 instead of resuming at its own 45. Either way it
// ends at its partition's block.
func TestAWholeRestartResumesFromTheHighestState(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

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

	var ts uint64
	send := func() {
		t.Helper()
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(10)

	// BVN0 node 2 stops executing: restarted and left collecting, it executes
	// nothing while the others go on.
	const behind = 2
	bvn0 := sim.S.Partition("BVN0")
	part := PartitionUrl("BVN0")
	bvn0.RestartNode(behind)
	for i := 0; i < 5; i++ {
		send()
	}
	sim.StepN(10)
	lag := partitionBlock(t, bvn0.NodeDatabase(behind), part)
	high := partitionBlock(t, bvn0.NodeDatabase(0), part)
	require.Less(t, lag, high, "precondition: node %d is behind", behind)

	var all []restartTarget
	var own []uint64
	for _, name := range []string{Directory, "BVN0"} {
		p := sim.S.Partition(name)
		for i := 0; i < 3; i++ {
			all = append(all, restartTarget{name, p, i})
			own = append(own, partitionBlock(t, p.NodeDatabase(i), PartitionUrl(name)))
		}
	}
	exits := restartAndJoin(t, sim, all)
	resumed := map[string]int{}
	for i, n := range all {
		if n.name == "BVN0" && n.node == behind {
			require.False(t, exits[i], "the node at block %d resumed from its own block while its peers held block %d", lag, high)
			continue
		}
		require.Equal(t, nodestate.StateActive, n.part.NodeJoinState(n.node).Machine().State(), "%s node %d is not ACTIVE", n.name, n.node)
		if exits[i] {
			resumed[n.name]++
			continue
		}
		// It found the threshold of its peers serving and joined from them:
		// from their state, not from one below its own.
		require.GreaterOrEqual(t, n.part.NodeJoinState(n.node).Overlap().From, own[i],
			"%s node %d joined from a state before its own block %d", n.name, n.node, own[i])
	}
	// Someone must resume: a node joins only from peers that serve, and only
	// once as many serve as sign an anchor, two of three. In BVN0 the node
	// behind serves nothing, so both of the others resume.
	for _, name := range []string{Directory, "BVN0"} {
		require.GreaterOrEqual(t, resumed[name], 2, "fewer %s validators resumed than sign an anchor", name)
	}

	// The node that was behind took its peers' state: it is ACTIVE from a
	// block at or past the block they held when it was behind, not from its
	// own. What its first blocks after the handoff then settle against the
	// block ledger is the overlap's (executor spec, "The handoff is an
	// overlap"; #4509), which the overlap tests run with the repair loop;
	// this test returns at the handoff, as restartAndJoin does.
	st := bvn0.NodeJoinState(behind)
	require.Equal(t, nodestate.StateActive, st.Machine().State(), "the node that was behind is not ACTIVE")
	require.GreaterOrEqual(t, st.Overlap().From, high, "the node that was behind joined from a state before its peers' block %d", high)

	// The validators that resumed agree, root for root.
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(10)
	block, root := partitionBlock(t, bvn0.NodeDatabase(0), part), bptRoot(t, bvn0.NodeDatabase(0))
	require.Equal(t, block, partitionBlock(t, bvn0.NodeDatabase(1), part), "BVN0 node 1 is at another block")
	require.Equal(t, root, bptRoot(t, bvn0.NodeDatabase(1)), "BVN0 node 1 has another root")

	// Every node, resumed or joined, is at its partition's block.
	for _, n := range all {
		p := PartitionUrl(n.name)
		require.Equal(t, partitionBlock(t, n.part.NodeDatabase(0), p), partitionBlock(t, n.part.NodeDatabase(n.node), p),
			"%s node %d is at another block than node 0", n.name, n.node)
	}
}
