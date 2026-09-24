// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// failingHandoffBuffer fails its first handoff the way the DAG service's does
// when a buffered group cannot be produced (collect.go performHandoff): the
// buffer is already taken and collecting mode already left.
type failingHandoffBuffer struct {
	fakeBuffer
	failures int
}

func (b *failingHandoffBuffer) Handoff(q uint64) error {
	if b.failures > 0 {
		b.failures--
		b.collecting = false
		return errors.UnknownError.WithFormat("produce buffered group 1 of 22 (round 1432): %w",
			errors.NotFound.With("Message.c06fcb7d….Main not found"))
	}
	return b.fakeBuffer.Handoff(q)
}

// Run 20260924T052134Z, acc-bvn3-val1's Directory: "Handoff failed; this node
// must join again", then "The join did not complete; this node is not
// executing", and no further join for the life of the process -- the node sat
// ACTIVE, not collecting and not executing, its lag climbing past 378 blocks.
//
// converge (join.go) retries a handoff refused NotReady or Conflict and
// returns every other error; Run returns it; the daemon's goroutine
// (cmd/accumulated/run/dagbft.go) logs it and exits. A handoff that failed
// after it took the buffer is exactly the case that has to start again.
func TestJoin_AFailedHandoffJoinsAgain(t *testing.T) {
	buf := &failingHandoffBuffer{failures: 1}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	outcome, err := run(t, Options{Partition: "Directory", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err, "a handoff that failed must send the node back to collecting and joining, not end the join")
	require.Equal(t, Joined, outcome)
	require.NotEmpty(t, buf.handoffs, "the node joined again and handed off")
	require.GreaterOrEqual(t, buf.starts, 2, "it started collecting again after the failure")

	// It matched and did not start executing from there, so it is not ACTIVE
	// (executor spec, "Sync", steps 5 and 6; #4385).
	require.Equal(t, []uint64{20}, state.demoted, "the failed handoff demotes the node at the block it matched")
}

// Every failed handoff is counted: the join retries without bound, so a
// failure that recurs on every attempt must show as a climbing count
// (executor spec, "Sync", step 5; #4401).
func TestJoin_AFailedHandoffIsCounted(t *testing.T) {
	const partition = "TestJoin_AFailedHandoffIsCounted"
	before := testutil.ToFloat64(mHandoffFailures.WithLabelValues(partition))

	buf := &failingHandoffBuffer{failures: 3}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: partition, Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []uint64{20}, buf.handoffs, "the fourth attempt hands off")
	require.Equal(t, 3.0, testutil.ToFloat64(mHandoffFailures.WithLabelValues(partition))-before,
		"each of the three failed attempts is counted")
}
