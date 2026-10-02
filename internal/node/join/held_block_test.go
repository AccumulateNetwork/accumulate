// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// heldBuffer is the DAG service's buffer as it stands when a block it was
// producing stayed held past its bound: a section of the state tree the block
// reads could not be loaded, so the service did not produce it, went back to
// collecting with the held group first in its buffer, and says which block it
// held (executor spec, "Sync", §3a; #4485 review, finding 3).
type heldBuffer struct {
	*fakeBuffer
	held uint64
}

func (b *heldBuffer) HeldBlock() uint64 { return b.held }

// holdingState is a node that hands off at b and executes nothing past it:
// block b + 1 is held. Its roots never diverge -- a held block commits
// nothing -- so the only thing that can tell the join is the buffer.
type holdingState struct {
	*divergingState
	buf   *heldBuffer
	asked int
}

func (s *holdingState) Diverged(context.Context) (uint64, bool, error) {
	switch {
	case len(s.buf.handoffs) == 0:
		return 0, false, nil
	case len(s.buf.handoffs) == 1:
		// The service holds block b + 1 past its bound and collects again.
		s.asked++
		if s.asked == 2 {
			s.buf.collecting = true
			s.buf.held = s.b + 1
		}
		return 0, false, nil
	default:
		s.stop()
		return 0, false, nil
	}
}

// TestJoin_ABlockHeldPastItsBoundSyncsAgain — #4485 review, finding 3. After
// the handoff, a block whose section could not be loaded is held; past the
// hold's bound the service collects again, holding that block's group, and
// the join must notice: it demotes the node at the held block, pulls again,
// and hands off again. A join that went on watching roots would leave the node
// ACTIVE and executing nothing for ever.
func TestJoin_ABlockHeldPastItsBoundSyncsAgain(t *testing.T) {
	const b = 20
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	buf := &heldBuffer{fakeBuffer: new(fakeBuffer)}
	stage := new(fakeStage)
	inner := &divergingState{buf: buf.fakeBuffer, b: b, diverged: b + 1, stop: cancel}
	state := &holdingState{divergingState: inner, buf: buf}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, _ = Run(ctx, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Retry: time.Millisecond})

	require.Equal(t, []uint64{b + 1}, inner.demoted, "the join did not demote the node at the block the service held")
	require.True(t, inner.demotedBeforePull, "the node was not demoted before it pulled again")
	require.True(t, inner.resynced, "the join did not pull again after the service held a block past its bound")
	require.Equal(t, []uint64{b, b + 1}, buf.handoffs, "the join did not hand off again")
}
