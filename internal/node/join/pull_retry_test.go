// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// A pull that fails does not end the join: the join pulls again and hands
// off. Run 20261003T182050Z: after a kill, bvn1-val3's Directory store
// refused its first writes, the pull failed, and the join returned — "The
// join did not complete; this node is not executing" — leaving the node
// collecting for good.
func TestJoin_APullThatFailsIsPulledAgainAndTheJoinHandsOff(t *testing.T) {
	const joiner = 2

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
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	submit := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 5; i++ {
		submit()
		sim.StepN(3)
	}
	sim.StepN(10)

	part := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	p.RestartNode(joiner)
	for i := 0; i < 3; i++ {
		submit()
		sim.StepN(3)
	}

	pulled, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  p.NodeDatabase(joiner),
		Sources: &join.QueryPeers{
			Client:  sim.S.Services(),
			Network: t.Name(),
			Router:  sim.S.Router(),
			Self:    p.NodePeerID(joiner),
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	buf := &countingBuffer{Buffer: p.NodeJoin(joiner)}
	state := &failingPulls{steppingPulledState: steppingPulledState{PulledState: pulled}, fail: 3}
	watches := 0
	state.step = func() {
		if state.steps%4 == 0 {
			submit()
		}
		state.steps++
		sim.Step()
	}
	state.watch = func() {
		watches++
		if len(buf.handoffs) > 0 && watches >= 5 || watches > 400 {
			cancel()
		}
	}

	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err = join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     state,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join does not end on a failed pull")
	require.Equal(t, 3, state.failed, "precondition: three pulls failed")
	require.Greater(t, state.pulls, 3, "the join pulled again after they failed")
	require.NotEmpty(t, buf.handoffs, "and handed off")
	require.False(t, p.Joining(joiner), "the node is executing")
}

// failingPulls fails its first pulls, as a store that refuses writes does.
type failingPulls struct {
	steppingPulledState
	fail, failed, pulls int
}

func (s *failingPulls) Pull(ctx context.Context) error {
	s.pulls++
	if s.failed < s.fail {
		s.failed++
		s.step()
		return errors.New("seal: shard 0: block 865 is already closed; now accumulating block 866")
	}
	return s.steppingPulledState.Pull(ctx)
}
