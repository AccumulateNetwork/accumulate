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
// root. The others resume from their own blocks as before.
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
	for _, name := range []string{Directory, "BVN0"} {
		p := sim.S.Partition(name)
		for i := 0; i < 3; i++ {
			all = append(all, restartTarget{name, p, i})
		}
	}
	exits := restartAndJoin(t, sim, all)
	for i, n := range all {
		if n.name == "BVN0" && n.node == behind {
			require.False(t, exits[i], "the node at block %d resumed from its own block while its peers held block %d", lag, high)
			continue
		}
		require.True(t, exits[i], "%s node %d did not resume from its own block", n.name, n.node)
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
}
