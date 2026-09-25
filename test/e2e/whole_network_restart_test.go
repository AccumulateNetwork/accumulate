// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
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

// TestAWholeNetworkRestartResumes — #4447. Every validator of every partition
// restarts at once, from its own store, and each runs the production join:
// join.Run over the state RestartNode builds as the daemon does, the node's
// own buffer and stage, and join.APIPeers over the simulator's services. Every
// peer a node asks is joining too, and refuses it NotReady, so there is no
// state to pull and no anchor to read. Each node executes from its own last
// block; the network produces blocks again and the validators' roots agree.
//
// Before #4447 each join looped for ever: "No peer could say which block the
// partition is at", as the 24h network's twelve validators did after a Docker
// daemon restart (2026-09-25T15:41Z).
//
// A validator whose store says an earlier pull started and did not hand off
// does not take that exit: it joins from its peers once they resume.
//
// Then one validator restarts alone, and it joins through the ordinary path:
// its peers are ACTIVE and answer, so the exit is not taken.
func TestAWholeNetworkRestartResumes(t *testing.T) {
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

	type target struct {
		name string
		part *simulator.Partition
		node int
	}
	var all []target
	for _, name := range []string{Directory, "BVN0"} {
		p := sim.S.Partition(name)
		for i := 0; i < 3; i++ {
			all = append(all, target{name, p, i})
		}
	}

	// runJoins restarts the nodes named and runs their joins together, as
	// their daemons would, while the network steps. The simulator is not
	// safe for a join and a step at once, so every call a join makes into
	// its node holds the lock a step holds. It returns, per node, whether it
	// took the whole-restart exit.
	runJoins := func(nodes []target) []bool {
		t.Helper()
		for _, n := range nodes {
			n.part.RestartNode(n.node)
			require.True(t, n.part.Joining(n.node))
		}
		var mu sync.Mutex
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		exited := make([]atomic.Bool, len(nodes))
		errs := make([]error, len(nodes))
		var wg sync.WaitGroup
		for i, n := range nodes {
			settler, ok := n.part.NodeExecutor(n.node).(join.Settler)
			require.True(t, ok)
			opts := join.Options{
				Partition: n.name,
				Buffer:    &lockedBuffer{Buffer: n.part.NodeJoin(n.node), mu: &mu},
				Stage: &lockedStage{Stage: &join.ExecutorStage{Settler: settler,
					Staging: n.part.NodeStaging(n.node), Database: n.part.NodeDatabase(n.node)}, mu: &mu},
				State: &lockedState{State: n.part.NodeJoinState(n.node), mu: &mu, exited: &exited[i]},
				Peers: &join.APIPeers{Partition: n.name, Client: sim.S.Services(), Network: t.Name()},
				Retry: time.Millisecond,
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				// The state is not a RootWatch through the wrapper, so Run
				// returns at the handoff.
				_, errs[i] = join.Run(ctx, opts)
			}()
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
	step:
		for {
			select {
			case <-done:
				break step
			default:
			}
			mu.Lock()
			err := sim.S.Step()
			mu.Unlock()
			require.NoError(t, err)
			time.Sleep(time.Millisecond)
		}
		out := make([]bool, len(nodes))
		for i, n := range nodes {
			require.NoError(t, errs[i], "%s node %d did not join", n.name, n.node)
			require.False(t, n.part.Joining(n.node), "%s node %d is still collecting", n.name, n.node)
			out[i] = exited[i].Load()
		}
		return out
	}

	active := func(n target) {
		t.Helper()
		require.Equal(t, nodestate.StateActive, n.part.NodeJoinState(n.node).Machine().State(),
			"%s node %d is not ACTIVE", n.name, n.node)
	}

	agree := func() {
		t.Helper()
		for _, name := range []string{Directory, "BVN0"} {
			p := sim.S.Partition(name)
			part := PartitionUrl(name)
			block, root := partitionBlock(t, p.NodeDatabase(0), part), bptRoot(t, p.NodeDatabase(0))
			for i := 1; i < 3; i++ {
				require.Equal(t, block, partitionBlock(t, p.NodeDatabase(i), part), "%s node %d is at another block", name, i)
				require.Equal(t, root, bptRoot(t, p.NodeDatabase(i)), "%s node %d has another root", name, i)
			}
		}
	}

	// One BVN0 validator stopped in the middle of an earlier pull: its store
	// holds some of a peer's accounts and not the rest, and says so. Its own
	// last block is no block's state, so it must not resume from it; it
	// joins from its peers once they have resumed.
	const halfPulled = 2
	bvn0 := sim.S.Partition("BVN0")
	require.NoError(t, bvn0.NodeDatabase(halfPulled).Update(func(batch *database.Batch) error {
		return batch.SystemData("BVN0").PullStarted().Put(partitionBlock(t, bvn0.NodeDatabase(halfPulled), PartitionUrl("BVN0")))
	}))

	// The whole network, at once.
	before := partitionBlock(t, bvn0.NodeDatabase(0), PartitionUrl("BVN0"))
	exits := runJoins(all)
	for i, n := range all {
		if n.name == "BVN0" && n.node == halfPulled {
			require.False(t, exits[i], "a node whose store says a pull started resumed from its own block")
			continue
		}
		require.True(t, exits[i], "%s node %d did not resume from its own block", n.name, n.node)
		active(n)
	}
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(10)
	require.Greater(t, partitionBlock(t, sim.S.Partition("BVN0").NodeDatabase(0), PartitionUrl("BVN0")), before,
		"the network did not produce a block after the restart")
	agree()

	// One validator, alone: its peers are ACTIVE, so it joins from them.
	alone := target{"BVN0", bvn0, 1}
	exits = runJoins([]target{alone})
	require.False(t, exits[0], "a validator restarted alone resumed from its own block instead of joining")
	active(alone)
	send()
	sim.StepN(10)
	agree()
}

// lockedState, lockedBuffer and lockedStage hold the simulator's lock for
// each call a join makes into its node. lockedState records whether the join
// took the whole-restart exit, which is the one caller of Executing.
type lockedState struct {
	join.State
	mu     *sync.Mutex
	exited *atomic.Bool
}

func (s *lockedState) Pull(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State.Pull(ctx)
}

func (s *lockedState) Matched(ctx context.Context) (uint64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State.Matched(ctx)
}

func (s *lockedState) Ready() (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State.Ready()
}

func (s *lockedState) Resumable() (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State.Resumable()
}

func (s *lockedState) Executing(block uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exited.Store(true)
	return s.State.Executing(block)
}

func (s *lockedState) Promote(block uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State.Promote(block)
}

func (s *lockedState) Demote(block uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State.Demote(block)
}

type lockedBuffer struct {
	join.Buffer
	mu *sync.Mutex
}

func (b *lockedBuffer) StartCollecting() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Buffer.StartCollecting()
}

func (b *lockedBuffer) StageThrough(block uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.StageThrough(block)
}

func (b *lockedBuffer) Handoff(q uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Handoff(q)
}

func (b *lockedBuffer) Resume() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Resume()
}

type lockedStage struct {
	join.Stage
	mu *sync.Mutex
}

func (s *lockedStage) SettleStagingAt(q uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Stage.SettleStagingAt(q)
}

func (s *lockedStage) HasGap(block uint64) ([]join.StreamGap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Stage.HasGap(block)
}
