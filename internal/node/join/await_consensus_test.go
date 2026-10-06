// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// orderingBuffer is a fakeBuffer whose consensus says how far it has
// collected: a round further each time it is asked, or, stuck, the same one.
type orderingBuffer struct {
	fakeBuffer
	stuck bool
	round uint64
}

func (b *orderingBuffer) CollectedRound() uint64 {
	if !b.stuck {
		b.round++
	}
	return b.round
}

func notCollectedYet(n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = errors.NotReady.With("the state is at a round this node has not collected to")
	}
	return errs
}

// A state consensus has not ordered up to is asked about again, without a
// pull round between, for as long as consensus is collecting. A node's
// consensus is seeded below the first state it proves and is at that state in
// seconds; pulling on meanwhile cost every join a round -- 16 to 55 s a
// partition in Docker run 20261006T044611Z -- to prove a state still further
// ahead of consensus (#4530).
func TestJoin_AsksAgainAboutTheSameStateWhileConsensusOrdersUpToIt(t *testing.T) {
	buf := &orderingBuffer{}
	buf.stageErrs = notCollectedYet(2)
	state := &fakeState{matchAt: 20, matchFrom: 1}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []uint64{21, 21, 21}, buf.staged, "the state was asked about until consensus had ordered up to it")
	require.Equal(t, []uint64{20}, buf.handoffs)
	require.Equal(t, 1, state.pulls, "the join pulled a round while consensus was ordering up to a state it had proven")
}

// A consensus that collects nothing is not waited on past the stall bound:
// the join pulls on, as it always did, and asks about the state it reaches.
func TestJoin_PullsOnWhenConsensusCollectsNothing(t *testing.T) {
	buf := &orderingBuffer{stuck: true}
	buf.stageErrs = notCollectedYet(40)
	state := &fakeState{matchAt: 20, matchFrom: 1}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers,
		AwaitConsensusStalled: 3 * time.Millisecond})
	require.NoError(t, err)
	require.Equal(t, []uint64{20}, buf.handoffs)
	require.Greater(t, state.pulls, 1, "the join waited, for good, on a consensus that was collecting nothing")
}

// Only a state consensus has not reached is asked about again. One the node
// cannot hand off from is not: asking again changes nothing, and the join
// pulls to a later state at once.
func TestJoin_ARefusedStateIsNotAskedAboutAgain(t *testing.T) {
	buf := &orderingBuffer{}
	buf.stageErrs = []error{errors.Conflict.With("the state is at or below the seed's floor")}
	state := &fakeState{matchAt: 20, matchFrom: 1}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []uint64{21, 21}, buf.staged)
	require.Equal(t, 2, state.pulls, "a refused state was asked about again instead of pulled past")
}

// The asking is bounded. A consensus that is collecting and is still short of
// the state when the bound is up is far behind for another reason; the join
// pulls on rather than hold one state for ever.
func TestJoin_StopsAskingAboutOneStateAfterItsBound(t *testing.T) {
	buf := &orderingBuffer{}
	buf.stageErrs = notCollectedYet(60)
	state := &fakeState{matchAt: 20, matchFrom: 1}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers,
		AwaitConsensus: 5 * time.Millisecond})
	require.NoError(t, err)
	require.Equal(t, []uint64{20}, buf.handoffs)
	require.Greater(t, state.pulls, 1, "the join held one state past its bound")
}
