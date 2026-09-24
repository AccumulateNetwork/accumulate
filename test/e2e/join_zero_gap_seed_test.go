// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multi "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAZeroGapRestartStillSeedsItsOwnBlocks (reviewer, #4400 note_3896114642).
//
// A node that restarts without falling behind joins like every other
// restart on this line (DIFFERENCES E11): Matched finds its own root in the
// anchored series at its own height H, and the join settles staging at H
// (join.go:341 -> SettleStaging -> synthCache().JoinedAt(H)). The node
// executed every block at or below H and holds every synthetic message it
// produced. The seed must still rebuild the in-flight window from the store
// -- that is what #4241/#4277 added it for -- or the restarted leader
// dispatches nothing when the Directory receipts a pre-restart block and its
// sequencer answers NotReady for blocks it produced.
func TestAZeroGapRestartStillSeedsItsOwnBlocks(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "Directory")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Cross-partition sends so BVN0's own synthetic chain grows.
	for i := uint64(1); i <= 5; i++ {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	// A burst whose synthetics are still in flight at the restart.
	for i := uint64(6); i <= 10; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
	}
	sim.StepN(2)

	bvn := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	h := partitionBlock(t, p.NodeDatabase(0), bvn)

	fresh := func(cache *synthcache.Cache) coreexec.Executor {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		x, err := multi.NewExecutor(coreexec.Options{
			Logger:        acctesting.NewTestLogger(t),
			Database:      p.NodeDatabase(0),
			Key:           priv,
			Router:        sim.S.Router(),
			EventBus:      events.NewBus(nil),
			NewDispatcher: func() coreexec.Dispatcher { return nopDispatcher{} },
			Sequencer:     sim.S.Services().Private(),
			Querier:       sim.S.Services(),
			Describe:      coreexec.DescribeShim{NetworkType: PartitionTypeBlockValidator, PartitionId: "BVN0"},
			Staging:       coreexec.NewStaging(),
			SynthCache:    cache,
		})
		require.NoError(t, err)
		return x
	}

	// Control: a fresh process that never joined (Joined() == 0) rebuilds the
	// in-flight window from its own store.
	control := synthcache.New(0)
	_, err := fresh(control).Begin(coreexec.BlockParams{Context: context.Background(), Index: h + 1, Time: time.Now()})
	require.NoError(t, err)
	cEntries, cBlocks := control.Len()
	t.Logf("control (no join): the seed rebuilt %d entries in %d blocks", cEntries, cBlocks)
	require.NotZero(t, cBlocks, "precondition: the node produced synthetics still in flight, and a seed without a join rebuilds them")

	// Production: the same process restarted at H, not behind. Its join
	// matches its own root at H and settles staging there (join.go:341).
	cache := synthcache.New(0)
	x := fresh(cache)
	require.NoError(t, x.(join.Settler).SettleStagingAt(h))
	require.Equal(t, h, cache.Joined())
	_, err = x.Begin(coreexec.BlockParams{Context: context.Background(), Index: h + 1, Time: time.Now()})
	require.NoError(t, err)
	entries, blocks := cache.Len()
	t.Logf("zero-gap restart (joined at %d): the seed rebuilt %d entries in %d blocks", h, entries, blocks)
	require.Equal(t, cBlocks, blocks, "a node that executed every block at or below the block it joined at must still seed its own in-flight blocks")
	// And the synthetics in them: a block this node executed has its
	// messages, and is never skipped as one it did not execute (#4400).
	require.NotZero(t, entries, "a node that executed its in-flight blocks rebuilt none of their synthetics")
	require.Equal(t, cEntries, entries, "a node that executed every block at or below the block it joined at must still seed its own in-flight synthetics")
}
