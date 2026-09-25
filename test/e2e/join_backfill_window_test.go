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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAJoinedNodeKeepsPaceWhileItBackfills — #4405, run 20260925T020517Z.
// A BVN3 follower handed off at 717, executed 718-731, and then produced
// nothing for ten minutes: every committed group failed at "begin block: drain
// delivery queues: load queued local delivery …: Message.….Main not found".
//
// Block 731 produced a local delivery, stored its message and queued it; block
// 732 drains the queue. Between the two the join's root watch backfilled the
// accounts it had taken by their chain heads, 64 a check, and every one of
// them is a commit of its own. BlockchainDB's window counts commits, and a
// shallow read of a record older than the window is answered absent: the
// message block 731 wrote was, to block 732, absent. The drain fails before
// the queue is cleared, so every later block fails the same way.
//
// Here the node joins on BlockchainDB at its minimum window, holding many
// accounts by their heads, and runs on with a local delivery in every block
// and the root watch between blocks, as the daemon runs it. It must keep
// producing at its peers' pace for 50 blocks.
func TestAJoinedNodeKeepsPaceWhileItBackfills(t *testing.T) {
	const extra = 200
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	dir := t.TempDir()
	open := func(partition *PartitionInfo, node int, _ logging.Logger) keyvalue.Beginner {
		db, err := bcdb.Open(filepath.Join(dir, partition.ID, fmt.Sprint(node)))
		require.NoError(t, err)
		require.NoError(t, db.SetMergeLag(20))
		t.Cleanup(func() { _ = db.Close() })
		return db
	}

	net, _ := networkWithAFollower(t.Name(), 1, 3)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
		simulator.WithDatabase(open),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	for i := 0; i < extra; i++ {
		MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath(fmt.Sprintf("t%d", i)), TokenUrl: AcmeUrl()})
	}

	part := sim.S.Partition("BVN0")
	partUrl := PartitionUrl("BVN0")
	const fresh = 3
	part.RestartNode(fresh)

	var ts uint64
	// A send from alice to bob: both are BVN0's, so the deposit is a local
	// delivery, queued at the end of the block and drained at the next.
	submit := func() {
		ts++
		env, err := build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey).
			Done()
		require.NoError(t, err)
		sim.SubmitTxnSuccessfully(env)
	}
	for i := 0; i < 5; i++ {
		submit()
		sim.StepN(3)
	}

	// Accounts the network changes before the node joins: its walk takes
	// each by its chain heads, and the root watch backfills them after the
	// handoff, a commit apiece -- more commits between two blocks than the
	// store's window.
	for i := 0; i < extra; i++ {
		ts++
		env, err := build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(alice, fmt.Sprintf("t%d", i)).
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey).
			Done()
		require.NoError(t, err)
		sim.SubmitTxnSuccessfully(env)
		if i%10 == 9 {
			sim.StepN(2)
		}
	}
	sim.StepN(10)

	part.RestartNode(fresh)
	state := part.NodeJoinState(fresh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{cancel: cancel, State: state, step: func(round int) {
		submit()
		sim.StepN(3)
		if round >= 200 {
			cancel()
		}
	}}
	settler, ok := part.NodeExecutor(fresh).(join.Settler)
	require.True(t, ok)
	_, _ = join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    part.NodeJoin(fresh),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: part.NodeStaging(fresh), Database: part.NodeDatabase(fresh)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.False(t, part.Joining(fresh), "the fresh node did not hand off")
	require.Equal(t, nodestate.StateActive, state.Machine().State(), "the fresh node is not ACTIVE")

	// Run on as the daemon does: a local delivery in every block, and the
	// root watch -- which backfills -- between blocks.
	var watch join.RootWatch = state
	joinedAt := partitionBlock(t, part.NodeDatabase(fresh), partUrl)
	headOnly := func() int {
		batch := part.NodeDatabase(fresh).Begin(false)
		defer batch.Discard()
		list, err := batch.SystemData("BVN0").HeadOnly().Get()
		require.NoError(t, err)
		return len(list)
	}
	require.Greater(t, headOnly(), 64, "precondition: the root watch has more than one check of backfilling left")
	for i := 0; i < 50; i++ {
		submit()
		sim.Step()
		_, diverged, err := watch.Diverged(context.Background())
		require.NoError(t, err)
		require.False(t, diverged, "block %d: the node's root diverged", partitionBlock(t, part.NodeDatabase(fresh), partUrl))
	}
	sim.StepN(3)

	at := partitionBlock(t, part.NodeDatabase(fresh), partUrl)
	peers := partitionBlock(t, part.NodeDatabase(0), partUrl)
	require.GreaterOrEqual(t, at, joinedAt+50, "the node produced %d blocks after its handoff at %d in 50 steps", at-joinedAt, joinedAt)
	require.Equal(t, peers, at, "the node is not at its peers' block")
	require.Equal(t, bptRoot(t, part.NodeDatabase(0)), bptRoot(t, part.NodeDatabase(fresh)), "the node's root is not its peers'")
}
