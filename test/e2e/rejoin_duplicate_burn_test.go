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

// TestARejoinedDirectoryAppliesABurnItsPeersSkip is #4433, failing on
// purpose. Soak 20260924T111811Z: acc-bvn1-val1's Directory rejoined by pull
// five times and diverged 3-14 blocks after each handoff, first in the BPT
// with the root chain equal (blocks 192, 223, 426), while every block matched
// its peers on round, arrived, batches and every stream position.
//
// The cause. A BVN block that bought credits sends the Directory one
// SyntheticBurnTokens carrying nothing but the amount (block_end.go,
// produceBlockMessages, "ACME burn (for credits)"): no cause, no source, no
// nonce. Two blocks, on any BVNs, that burned the same amount therefore send
// the SAME transaction, hash for hash. The Directory executes the first and
// every later one finds Transaction(H).Status delivered and is not processed
// (msg_transaction.go, executeTransaction) -- on a node that holds that
// status. A node that joined by pull does not hold it for any transaction
// executed while it was away: the pull brings account state and the chains,
// never transaction statuses. So when an amount first burned during the gap
// is burned again after the handoff, the peers skip it and the joined node
// executes it: ACME's Issued drops again on that node alone. ACME's main
// chain is not appended, because the chain's element index -- which the pull
// did bring -- says the hash is already on it, so the root chain is the
// peers' and only ACME's leaf, and with it the BPT, differs. That is the
// soak's shape exactly; where the element index is missing the chain is
// appended too and the root differs in the same block (433, 479).
//
// Fails on demand: the two nodes' BPT roots differ within a few blocks of
// the second burn, with the root chains equal at that block and ACME the one
// leaf that differs.
var ctlAmount = 3

func TestARejoinedDirectoryAppliesABurnItsPeersSkip(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e15))

	dnPart := sim.S.Partition(Directory)
	dn := DnUrl()
	const joiner, peer = 0, 1

	// buyCredits submits n identical credit purchases into one BVN0 block,
	// so that block burns n ACME and sends the Directory one burn of n.
	var ts uint64
	buyCredits := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			ts++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					AddCredits().Spend(1).To(alice, "book", "1").WithOracle(InitialAcmeOracle).
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		}
		sim.StepN(1)
	}
	issued := func(node int) *big.Int {
		var acme *TokenIssuer
		View(t, dnPart.NodeDatabase(node), func(batch *database.Batch) {
			require.NoError(t, batch.Account(AcmeUrl()).Main().GetAs(&acme))
		})
		return &acme.Issued
	}
	rootAnchor := func(node int) [32]byte {
		var a [32]byte
		View(t, dnPart.NodeDatabase(node), func(batch *database.Batch) {
			head, err := batch.Account(dn.JoinPath(Ledger)).RootChain().Head().Get()
			require.NoError(t, err)
			a = *(*[32]byte)(head.Anchor())
		})
		return a
	}

	// Before the restart every Directory node executes a burn of 1.
	for i := 0; i < 3; i++ {
		buyCredits(1)
		sim.StepN(5)
	}

	// The joiner stops. While it is away BVN0 burns 2 in one block for the
	// first time, and its peers execute that burn.
	dnPart.RestartNode(joiner)
	require.True(t, dnPart.Joining(joiner))
	before := issued(peer)
	buyCredits(2)
	sim.StepN(10)
	require.NotEqual(t, before.String(), issued(peer).String(),
		"premise: the peers executed the gap's burn of 2")

	// It rejoins by the production pull, as run 20260924T111811Z's did.
	dnPart.RestartNode(joiner)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{State: dnPart.NodeJoinState(joiner), step: func(round int) {
		sim.StepN(1)
		if round >= 200 {
			cancel()
		}
	}}
	settler, ok := dnPart.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	outcome, err := join.Run(ctx, join.Options{
		Partition: Directory,
		Buffer:    dnPart.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: dnPart.NodeStaging(joiner), Database: dnPart.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: Directory, Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, join.Joined, outcome)
	joinedAt := partitionBlock(t, dnPart.NodeDatabase(joiner), dn)
	require.Equal(t, partitionBlock(t, dnPart.NodeDatabase(peer), dn), joinedAt)
	require.Equal(t, bptRoot(t, dnPart.NodeDatabase(peer)), bptRoot(t, dnPart.NodeDatabase(joiner)),
		"premise: the joiner hands off at its peers' root")
	require.Equal(t, issued(peer).String(), issued(joiner).String(), "premise: ACME agrees at the handoff")

	// Premise: the joiner holds none of the gap's transaction statuses. The
	// burn of 2 is on ACME's main chain on both nodes; only the peer knows it
	// executed.
	burn := new(Transaction)
	burn.Header.Principal = AcmeUrl()
	burn.Body = &SyntheticBurnTokens{Amount: *big.NewInt(2 * AcmePrecision)}
	for _, n := range []int{joiner, peer} {
		View(t, dnPart.NodeDatabase(n), func(batch *database.Batch) {
			_, err := batch.Account(AcmeUrl()).MainChain().Inner().ElementIndex(burn.GetHash()).Get()
			require.NoError(t, err, "premise: node %d holds the burn of 2 on ACME's main chain", n)
		})
	}
	status := func(n int) bool {
		var delivered bool
		View(t, dnPart.NodeDatabase(n), func(batch *database.Batch) {
			st, err := batch.Transaction(burn.GetHash()).Status().Get()
			require.NoError(t, err)
			delivered = st.Delivered()
		})
		return delivered
	}
	require.True(t, status(peer), "premise: the peer recorded the burn of 2 as delivered")
	require.False(t, status(joiner), "premise: the joiner holds no status for the gap's burn of 2")

	// After the handoff BVN0 burns 2 again: the same transaction.
	buyCredits(ctlAmount)
	for i := 0; i < 15; i++ {
		sim.StepN(1)
		b := partitionBlock(t, dnPart.NodeDatabase(peer), dn)
		require.Equal(t, b, partitionBlock(t, dnPart.NodeDatabase(joiner), dn))
		if bptRoot(t, dnPart.NodeDatabase(joiner)) == bptRoot(t, dnPart.NodeDatabase(peer)) {
			continue
		}
		differ := []string{}
		lj, lp := leaves(t, dnPart.NodeDatabase(joiner)), leaves(t, dnPart.NodeDatabase(peer))
		for k, v := range lp {
			if lj[k] != v {
				differ = append(differ, k)
			}
		}
		t.Logf("block %d, %d after the handoff at %d: root chains equal %v; leaves that differ %v; ACME issued joiner %v, peer %v",
			b, b-joinedAt, joinedAt, rootAnchor(joiner) == rootAnchor(peer), differ, issued(joiner), issued(peer))
		require.Failf(t, "the rejoined Directory diverged",
			"block %d: the joiner executed a burn its peers skipped as already delivered", b)
	}
}
