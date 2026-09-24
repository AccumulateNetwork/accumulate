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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAFreshBVNNodeJoinsByPull — #4421. A BVN0 node with a genesis store
// joins a running network by the production join.Run and hands off.
//
// On issue-4205-lead @ b67bf3b21 it pulls and then fails every handoff:
//
//	produce buffered block N: begin block: seed synthetic cache: load
//	Directory receipts: load anchor pool main chain entry K: Message.… not found
//
// The fresh node takes bvn-BVN0.acme/anchors in full (with the message behind
// every entry) in the spine pass. Every later pass names the pool again -- the
// pool is written every block, so the block ledger names it -- and fetchPass
// skips the spine accounts only in the spine pass, so the later passes pull the
// pool STATE-ONLY: chain heads and the open mark set's entries, no message
// behind any of them (internal/node/join/state.go fetchPass/fetchOne,
// internal/core/bootstrap/pull/pull.go pullChainHeads). The pool's main chain
// then holds entries past the spine pass with no message, and the seed reads
// the message behind every newest entry with no skip (ownReceipts,
// internal/core/execute/v2/block/synth_cache_seed.go).
func TestAFreshBVNNodeJoinsByPull(t *testing.T) {
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

	bvnPart := sim.S.Partition("BVN0")
	const fresh = 3 // networkWithAFollower appends it after the validators
	require.Equal(t, 4, bvnPart.NodeCount())

	// The fresh node: its BVN0 half starts its join before any block, so it
	// has executed nothing beyond genesis.
	bvnPart.RestartNode(fresh)

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

	// A process start: what the node collected is dropped, so it holds
	// genesis and nothing after it, and must pull.
	bvnPart.RestartNode(fresh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 200
	stepping := &steppingState{State: bvnPart.NodeJoinState(fresh), step: func(round int) {
		if round%5 == 0 {
			send()
		}
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}
	settler, ok := bvnPart.NodeExecutor(fresh).(join.Settler)
	require.True(t, ok)
	outcome, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    bvnPart.NodeJoin(fresh),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: bvnPart.NodeStaging(fresh), Database: bvnPart.NodeDatabase(fresh)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})

	// Which entries of the pool's main chain the fresh node holds without the
	// message behind them.
	pool := PartitionUrl("BVN0").JoinPath(AnchorPool)
	var missing []int64
	var count int64
	View(t, bvnPart.NodeDatabase(fresh), func(batch *database.Batch) {
		c := batch.Account(pool).MainChain()
		head, err := c.Head().Get()
		require.NoError(t, err)
		count = head.Count
		for i := int64(0); i < head.Count; i++ {
			h, err := c.Entry(i)
			if err != nil {
				missing = append(missing, i)
				continue
			}
			var msg messaging.Message
			if batch.Message2(h).Main().GetAs(&msg) != nil {
				missing = append(missing, i)
			}
		}
	})
	t.Logf("fresh BVN0 node: %v main chain has %d entries, %d without a message: %v", pool, count, len(missing), missing)

	require.NoError(t, err, "a fresh BVN0 node did not join within %d pull rounds", maxRounds)
	require.Equal(t, join.Joined, outcome)
	require.False(t, bvnPart.Joining(fresh))
	require.Empty(t, missing, "the joined node holds pool entries with no message behind them")
}
