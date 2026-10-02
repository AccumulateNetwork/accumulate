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

// watchedState is the state of a node that started with its network: it
// executes from its own blocks, is checked, diverges once at block diverged,
// and is synced again at that block when the join pulls while collecting.
type watchedState struct {
	buf      *fakeBuffer
	diverged uint64
	stop     context.CancelFunc

	executing []uint64 // every block the watch marked the node executing from
	asked     int      // how many times Diverged was asked
	synced    uint64   // the block the pull synced to; zero until it did
	promoted  []uint64
	demoted   []uint64
}

func (s *watchedState) Executing(block uint64) error {
	s.executing = append(s.executing, block)
	return nil
}
func (s *watchedState) Promote(block uint64)    { s.promoted = append(s.promoted, block) }
func (s *watchedState) Demote(block uint64)     { s.demoted = append(s.demoted, block) }
func (*watchedState) Resumable() (uint64, bool) { return 0, false }
func (*watchedState) Backfill(context.Context)  {}
func (*watchedState) Ready() (uint64, bool)     { return 0, false }

func (s *watchedState) Pull(context.Context) error {
	if s.buf.collecting && len(s.demoted) > 0 {
		s.synced = s.diverged
	}
	return nil
}

func (s *watchedState) Matched(context.Context) (uint64, bool, error) {
	return s.synced, s.synced > 0, nil
}

func (s *watchedState) Diverged(context.Context) (uint64, bool, error) {
	s.asked++
	if s.synced == 0 {
		return s.diverged, true, nil
	}
	if len(s.buf.handoffs) > 0 {
		s.stop() // synced and handed off: the test has seen what it needs
	}
	return 0, false, nil
}

// TestAFollowerStartedWithTheNetworkIsWatchedAndRepaired: a non-validator
// that executes from genesis never joins, so nothing checked it (#4506).
// Watch marks it executing from its own block, checks every executed block
// against its signed anchor as the join does after a handoff, and on a
// mismatch demotes it, syncs it from the ledger and hands it off again.
func TestAFollowerStartedWithTheNetworkIsWatchedAndRepaired(t *testing.T) {
	const genesis, diverged = 1, 30
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &watchedState{buf: buf, diverged: diverged, stop: cancel}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := Watch(ctx, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Retry: time.Millisecond}, genesis)
	require.NoError(t, err)

	require.Equal(t, []uint64{genesis}, state.executing, "the node is ACTIVE on its own root from its first block, with no pull and no handoff")
	require.NotZero(t, state.asked, "its executed blocks are checked against their proven roots")
	require.Equal(t, []uint64{diverged}, state.demoted, "a mismatch demotes it at the diverged block")
	require.Equal(t, 1, buf.starts, "and it starts collecting, once, there")
	require.Equal(t, uint64(diverged), state.synced, "it syncs to the diverged block's anchored state")
	require.Equal(t, []uint64{diverged}, buf.handoffs, "and hands off there, its first handoff")
	require.Equal(t, []uint64{diverged}, state.promoted, "promoted at that handoff and nowhere else")
	require.Equal(t, uint64(diverged), stage.settled, "staging settles at the block it synced to")
}
