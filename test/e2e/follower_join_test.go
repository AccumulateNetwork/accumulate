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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAFollowerJoinsARunningNetworkAndLeaves: a node whose key is in no
// committee joins a running network under load, and leaves it (#4363).
//
// The join is production's: join.Run over the node's own buffer, its own
// executor's stage, and join.PulledState through join.QueryPeers with its own
// peer ID excluded (#4303), as cmd/accumulated/run/dagbft.go wires it. Nothing
// is copied from a peer's store (CompleteJoin is not used), and nothing here
// does a step the daemon must do for itself.
//
// The follower is in the network from genesis, because the simulator cannot
// add a node to a running network. It is sent into its join before the first
// block, so it has executed nothing beyond genesis — what a fresh node holds —
// and the network runs on, under load, while it collects.
//
// What it asserts, each where production would see it:
//
//   - a read addressed to the follower before it is ACTIVE answers NotReady,
//     and the same read after answers, with its peers' value;
//   - a transaction submitted to the follower while it is still joining is
//     executed by the committee — read from a committee node's store, never
//     from what the follower answered;
//   - after the join it executes, at its peers' block and root, on the same
//     root chain the Directory anchors;
//   - stopping it leaves the partition's block cadence unchanged.
func TestAFollowerJoinsARunningNetworkAndLeaves(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	net, followerKey := networkWithAFollower(t.Name(), 1, 3)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		// A joining node executes nothing, so it reports no result for the
		// blocks it collects; that is not a consensus failure.
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

	// The premise: the follower's key is active on nothing.
	def := sim.NetworkStatus(api.NetworkStatusOptions{Partition: Directory}).Network
	_, info, ok := def.ValidatorByKey(followerKey[32:])
	require.True(t, ok, "the follower's key is not in the definition")
	for _, part := range def.Partitions {
		require.Falsef(t, info.IsActiveOn(part.ID),
			"the follower is active on %s — it is a validator, not a follower", part.ID)
	}

	part := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	const follower = 3 // networkWithAFollower appends it after the validators
	require.Equal(t, follower+1, p.NodeCount())
	followerID := p.NodePeerID(follower)

	// The follower starts its join before any block: it has executed nothing
	// beyond genesis and collects every block from here.
	p.RestartNode(follower)
	require.True(t, p.Joining(follower))
	fresh := partitionBlock(t, p.NodeDatabase(follower), part)
	require.LessOrEqual(t, fresh, uint64(GenesisBlock),
		"precondition: the follower has executed no block of its own")

	var ts uint64
	send := func() *messaging.Envelope {
		ts++
		env, err := build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey).
			Done()
		require.NoError(t, err)
		return env
	}

	// The network is running, and loaded, before the follower asks anyone.
	for i := 0; i < 5; i++ {
		st := sim.SubmitTxnSuccessfully(send())
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(10)
	require.Greater(t, partitionBlock(t, p.NodeDatabase(0), part), fresh,
		"precondition: the network is running")

	ctx := context.Background()
	followerQuery := sim.S.Services().ForPeer(followerID).
		ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr())
	followerSubmit := sim.S.Services().ForPeer(followerID).
		ForAddress(api.ServiceTypeSubmit.AddressFor("BVN0").Multiaddr())

	// A read sent to the follower before it is ACTIVE is refused, so the
	// caller asks another node. Its store holds genesis and whatever the
	// pull has filled; neither is state it executed.
	_, err := followerQuery.Query(ctx, alice.JoinPath("tokens"), &api.DefaultQuery{})
	require.Error(t, err, "the follower answered a read while it was joining")
	require.Equal(t, errors.NotReady, errors.Code(err),
		"a joining follower must refuse a read as NotReady, got %v", err)

	// The join, as the daemon runs it. The simulator has no clock, so the
	// network steps — under load — after every pull round. In the first round
	// a transaction is handed to the follower itself, while it is joining.
	// The state is the one RestartNode built as the daemon builds it, and
	// its machine is what the follower's querier refuses by: a state built
	// here would be one the follower's services never see.
	state := p.NodeJoinState(follower)
	require.NotNil(t, state, "the restarted follower has no join state")

	// The peers the join itself pulls from, not a set built beside it: the
	// follower must never be one of them (#4303).
	srcs, srcPart, err := state.Sources().For(ctx, alice.JoinPath("tokens"))
	require.NoError(t, err)
	require.True(t, part.Equal(srcPart), "the account routed to %v, not %v", srcPart, part)
	require.Len(t, srcs, p.NodeCount()-1, "the follower must be excluded from its own peer list")

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	const maxRounds = 200
	var relayed *url.TxID
	var joiningRounds, matchedRounds int
	stepping := &steppingState{State: state, step: func(round int) {
		// A node that has not handed off executes nothing and is BOOTING,
		// whether or not its root has matched yet (#4385: ACTIVE at the
		// match served stale anchors to the next joiner, #4413).
		if p.Joining(follower) {
			joiningRounds++
			if _, ok, _ := state.Matched(ctx); ok {
				matchedRounds++
			}
			require.Equal(t, nodestate.StateBooting, state.Machine().State(),
				"round %d: the follower is still joining and reads %v", round, state.Machine().State())
		}
		if round == 1 {
			require.True(t, p.Joining(follower), "precondition: the follower is still joining")
			env := send()
			relayed = env.Transaction[0].ID()
			subs, err := followerSubmit.Submit(ctx, env, api.SubmitOptions{})
			require.NoError(t, err, "the follower refused a transaction while it was joining")
			require.NotEmpty(t, subs)
		} else {
			sim.SubmitTxnSuccessfully(send())
		}
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}
	settler, ok := p.NodeExecutor(follower).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	outcome, err := join.Run(runCtx, join.Options{
		Partition: "BVN0",
		Buffer:    p.NodeJoin(follower),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(follower), Database: p.NodeDatabase(follower)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the follower did not join within %d pull rounds", maxRounds)
	require.Equal(t, join.Joined, outcome)
	require.False(t, p.Joining(follower), "the joined follower executes")
	require.Empty(t, entriesWithNoMessage(t, p.NodeDatabase(follower), "BVN0"), "the joined follower holds spine entries with no message behind them (#4421)")
	require.Equal(t, nodestate.StateActive, state.Machine().State(), "the follower did not promote at its handoff")
	t.Logf("the follower read BOOTING on all %d rounds it was joining, %d of them after its root had matched",
		joiningRounds, matchedRounds)

	// The transaction handed to the follower while it joined was executed by
	// the committee: read at the destination, from a validator's store.
	require.NotNil(t, relayed)
	executed := func() bool {
		batch := p.NodeDatabase(0).Begin(false)
		defer batch.Discard()
		h := relayed.Hash()
		st, err := batch.Transaction(h[:]).Status().Get()
		return err == nil && st.Delivered() && st.Error == nil
	}
	for i := 0; i < 50 && !executed(); i++ {
		sim.Step()
	}
	require.True(t, executed(),
		"the transaction submitted to the joining follower was never executed by the committee")

	// It executes: with more load it moves with its peers and stands at their
	// block, their root and their root chain — the chain the Directory anchors.
	joinedAt := partitionBlock(t, p.NodeDatabase(follower), part)
	followerRoots := bptChainHeight(t, p.NodeDatabase(follower), func(batch *database.Batch) *database.Chain2 {
		return batch.Account(part.JoinPath(Ledger)).BptChain()
	})
	anchoredRoots := bptChainHeight(t, sim.S.Partition(Directory).NodeDatabase(0), dnBVN0BptChain)
	for i := 0; i < 5; i++ {
		st := sim.SubmitTxnSuccessfully(send())
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(10)
	require.Greater(t, partitionBlock(t, p.NodeDatabase(follower), part), joinedAt,
		"the joined follower does not execute")
	require.Equal(t, partitionBlock(t, p.NodeDatabase(0), part), partitionBlock(t, p.NodeDatabase(follower), part),
		"the follower does not stand at its peers' block")
	require.Equal(t, bptRoot(t, p.NodeDatabase(0)), bptRoot(t, p.NodeDatabase(follower)),
		"the follower's state root is not its peers'")
	rootChain := func(i int) []byte {
		var a []byte
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			var err error
			a, err = batch.Account(part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
		})
		return a
	}
	require.Equal(t, rootChain(0), rootChain(follower),
		"the follower's root chain diverged from the committee's")

	// And a root the Directory anchored after the join is one the follower
	// produced itself: the latest BVN0 state root on the Directory's anchor
	// chain is among the roots the follower's own bpt chain gained since it
	// handed off.
	dn := sim.S.Partition(Directory).NodeDatabase(0)
	require.Greater(t, bptChainHeight(t, dn, dnBVN0BptChain), anchoredRoots,
		"the Directory anchored no BVN0 root after the follower joined")
	var anchored []byte
	var produced [][]byte
	View(t, dn, func(batch *database.Batch) {
		chain := dnBVN0BptChain(batch)
		head, err := chain.Head().Get()
		require.NoError(t, err)
		anchored, err = chain.Entry(head.Count - 1)
		require.NoError(t, err)
	})
	View(t, p.NodeDatabase(follower), func(batch *database.Batch) {
		chain := batch.Account(part.JoinPath(Ledger)).BptChain()
		head, err := chain.Head().Get()
		require.NoError(t, err)
		for i := followerRoots; i < head.Count; i++ {
			e, err := chain.Entry(i)
			require.NoError(t, err)
			produced = append(produced, e)
		}
	})
	require.Contains(t, produced, anchored,
		"the BVN0 root the Directory anchored last, %x, is not one the follower produced after it joined", anchored)

	// The same read, sent to the same node, now answers — with its peers' value.
	rec, err := followerQuery.Query(ctx, alice.JoinPath("tokens"), &api.DefaultQuery{})
	require.NoError(t, err, "the ACTIVE follower refused a read")
	peerRec, err := sim.S.Services().ForPeer(p.NodePeerID(0)).
		ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr()).
		Query(ctx, alice.JoinPath("tokens"), &api.DefaultQuery{})
	require.NoError(t, err)
	require.Equal(t,
		peerRec.(*api.AccountRecord).Account.(*TokenAccount).Balance.String(),
		rec.(*api.AccountRecord).Account.(*TokenAccount).Balance.String(),
		"the follower answers a different balance than its peers")

	// It leaves. The simulator has no way to stop a node today; the seam this
	// asks for is the smallest one: stop node i — it is handed no block,
	// answers nothing and casts no vote from here on.
	stopper, ok := any(p).(interface{ StopNode(int) })
	require.True(t, ok, "the simulator cannot stop a node: (*simulator.Partition).StopNode(int) does not exist")

	const window = 20
	cadence := func() uint64 {
		// The ledger records the last block that executed something, so
		// after empty blocks it stands behind the partition's height. One
		// loaded step first puts it at the height, or the window would count
		// the empty blocks before it too (24 blocks in 20 steps, measured).
		sim.SubmitTxnSuccessfully(send())
		sim.Step()
		from := partitionBlock(t, p.NodeDatabase(0), part)
		for i := 0; i < window; i++ {
			sim.SubmitTxnSuccessfully(send())
			sim.Step()
		}
		return partitionBlock(t, p.NodeDatabase(0), part) - from
	}
	before := cadence()
	require.NotZero(t, before, "precondition: the partition produces blocks")
	require.Equal(t, partitionBlock(t, p.NodeDatabase(0), part), partitionBlock(t, p.NodeDatabase(follower), part),
		"precondition: the follower executes with its peers until it stops")
	stopper.StopNode(follower)
	stoppedAt := partitionBlock(t, p.NodeDatabase(follower), part)
	after := cadence()

	// What a stop is: the follower executes nothing more while its peers go on,
	// and a call addressed to it finds no one.
	require.Greater(t, partitionBlock(t, p.NodeDatabase(0), part), stoppedAt,
		"precondition: the partition went on after the follower stopped")
	require.Equal(t, stoppedAt, partitionBlock(t, p.NodeDatabase(follower), part),
		"the stopped follower went on executing blocks")
	_, err = followerQuery.Query(ctx, alice.JoinPath("tokens"), &api.DefaultQuery{})
	require.Error(t, err, "the stopped follower answered a read")
	require.Equal(t, errors.NoPeer, errors.Code(err), "a stopped follower must be absent, got %v", err)

	// And the partition did not notice. In the simulator one step is one
	// block on every partition whoever is running, so this shows only that
	// the partition runs on; whether a real committee's cadence changes when a
	// follower leaves is the Docker half's measurement (#4363).
	require.Equal(t, before, after,
		"stopping the follower changed the partition's block cadence: %d blocks in %d steps before, %d after",
		before, window, after)
}

// dnBVN0BptChain is the Directory's record of BVN0's state roots: the bpt
// half of its anchor chain for BVN0.
func dnBVN0BptChain(batch *database.Batch) *database.Chain2 {
	return batch.Account(DnUrl().JoinPath(AnchorPool)).AnchorChain("BVN0").BPT()
}

// bptChainHeight is how many entries a chain holds, zero when it has none.
func bptChainHeight(t *testing.T, db *database.Database, chain func(*database.Batch) *database.Chain2) int64 {
	t.Helper()
	var n int64
	View(t, db, func(batch *database.Batch) {
		head, err := chain(batch).Head().Get()
		require.NoError(t, err)
		n = head.Count
	})
	return n
}
