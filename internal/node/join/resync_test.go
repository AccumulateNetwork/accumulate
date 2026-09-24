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

// divergingState is a state pull whose node executes a block whose root does
// not match its proven root. It syncs to b on its first pull; after the first
// handoff, the executed block diverged has a local root that differs from the
// newest proven root covering it. The proof for a block arrives blocks later
// through DN anchors, so "not proven yet" and "proven and different" are two
// answers: Matched cannot tell them apart, Diverged can.
//
// A pull made while the node is collecting again after the handoff re-syncs
// at the diverged block: it brings the state to that block's anchored state.
type divergingState struct {
	buf      *fakeBuffer
	b        uint64
	diverged uint64
	stop     context.CancelFunc

	synced   uint64 // zero until the first pull
	resynced bool

	divergedAsked      int  // how many times Diverged was asked after the first handoff
	collectingAtRePull bool // whether the node was collecting when it pulled after the handoff

	promoted          []uint64 // every block the join promoted the node at
	demoted           []uint64 // every block the join demoted the node at
	demotedBeforePull bool     // whether the node was demoted when it pulled after the handoff
}

func (s *divergingState) Promote(block uint64) { s.promoted = append(s.promoted, block) }
func (s *divergingState) Demote(block uint64)  { s.demoted = append(s.demoted, block) }

func (s *divergingState) Pull(context.Context) error {
	switch {
	case s.synced == 0:
		s.synced = s.b
	case len(s.buf.handoffs) > 0 && !s.resynced:
		s.collectingAtRePull = s.buf.collecting
		s.demotedBeforePull = len(s.demoted) > 0
		if s.buf.collecting {
			s.synced = s.diverged
			s.resynced = true
		}
	}
	return nil
}

// Ready: this fake's state is never executed from before it matches.
func (s *divergingState) Ready() (uint64, bool) { return 0, false }

func (s *divergingState) Matched(context.Context) (uint64, bool, error) {
	return s.synced, s.synced > 0, nil
}

// Diverged reports an executed block whose local root differs from the newest
// proven root that covers it, and whether there is one.
func (s *divergingState) Diverged(context.Context) (uint64, bool, error) {
	switch {
	case len(s.buf.handoffs) == 0:
		return 0, false, nil
	case !s.resynced:
		s.divergedAsked++
		return s.diverged, true, nil
	default:
		// Re-synced and handed off again: the executed blocks match. The
		// test has seen what it needs; a join that keeps watching stops here.
		if len(s.buf.handoffs) > 1 {
			s.stop()
		}
		return 0, false, nil
	}
}

// After executing any block, the local root equals that block's proven root or
// it does not. A mismatch is a gap the sequence check missed: the node
// re-syncs at that block and continues, so a wrong run is caught at the block
// it happens in and never carried forward (executor spec, "Sync"; PLAN E11
// #4362, "root match after every executed block").
//
// The join hands off at B, executes B+1 and B+2, and B+2's root differs from
// the proven root that covers it. The join must not stay handed off: it
// collects again, pulls the anchored state at B+2, asks whether B+3 has a gap
// and, none, settles staging at B+2 and hands off there. No peer is asked for
// its staging.
func TestJoin_ReSyncsWhenAnExecutedBlocksRootDoesNotMatchTheProvenRoot(t *testing.T) {
	const b = 20
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &divergingState{buf: buf, b: b, diverged: b + 2, stop: cancel}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, _ = Run(ctx, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Retry: time.Millisecond})

	require.NotEmpty(t, buf.handoffs, "the join hands off")
	require.Equal(t, uint64(b), buf.handoffs[0], "the join hands off first at B")
	require.NotZero(t, state.divergedAsked,
		"after the handoff the join checks the executed blocks' roots against the proven root")
	require.Equal(t, 2, buf.starts, "on the mismatch the node starts collecting again")
	require.True(t, state.collectingAtRePull, "and it is collecting when it re-syncs")
	require.True(t, state.resynced, "the join re-syncs at the diverged block")
	require.Equal(t, []uint64{b, b + 2}, buf.handoffs,
		"and hands off again at the diverged block, whose anchored state it pulled")
	require.Equal(t, uint64(b+2), stage.settled, "staging settles at the block it re-synced to")
	require.Contains(t, stage.gapAsked, uint64(b+3), "the block after the re-sync is checked for a gap")
	require.Empty(t, peers.asked, "no peer is asked for its staging")
	require.Nil(t, stage.loaded)

	// A node whose root is known wrong is not ACTIVE (executor spec, "Sync",
	// steps 4 and 6; #4385): the join demotes it at the block that diverged,
	// before it pulls again, and once only.
	require.Equal(t, []uint64{b + 2}, state.demoted, "the re-sync demotes the node at the diverged block")
	require.True(t, state.demotedBeforePull, "and it is demoted before it syncs again")
	require.Equal(t, []uint64{b, b + 2}, state.promoted, "and promoted at each handoff, and nowhere else")
}
